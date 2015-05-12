package lifecycle_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Denying access to network ranges", func() {
	var (
		blockedListener   garden.Container
		blockedListenerIP string

		unblockedListener   garden.Container
		unblockedListenerIP string

		allowedListener   garden.Container
		allowedListenerIP string

		sender garden.Container
	)

	BeforeEach(func() {
		client = startGarden()

		var err error

		// create a listener to which we deny network access
		blockedListener, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
		blockedListenerIP = containerIP(blockedListener)

		// create a listener to which we do not deny access
		unblockedListener, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
		unblockedListenerIP = containerIP(unblockedListener)

		// create a listener to which we exclicitly allow access
		allowedListener, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
		allowedListenerIP = containerIP(allowedListener)

		restartGarden(
			"-denyNetworks", strings.Join([]string{
				blockedListenerIP + "/32",
				allowedListenerIP + "/32",
			}, ","),
			"-allowNetworks", allowedListenerIP+"/32",
		)

		// check that the IPs were preserved over restart
		Expect(containerIP(blockedListener)).To(Equal(blockedListenerIP))
		Expect(containerIP(unblockedListener)).To(Equal(unblockedListenerIP))
		Expect(containerIP(allowedListener)).To(Equal(allowedListenerIP))

		// create a container with the new deny network configuration
		sender, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(sender.Handle())
		Expect(err).ToNot(HaveOccurred())

		err = client.Destroy(blockedListener.Handle())
		Expect(err).ToNot(HaveOccurred())

		err = client.Destroy(unblockedListener.Handle())
		Expect(err).ToNot(HaveOccurred())

		err = client.Destroy(allowedListener.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	runInContainer := func(container garden.Container, script string) garden.Process {
		process, err := container.Run(garden.ProcessSpec{
			User: "root",
			Path: "sh",
			Args: []string{"-c", script},
		}, garden.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})
		Expect(err).ToNot(HaveOccurred())

		return process
	}

	It("makes that block of ip addresses inaccessible to the container", func() {
		runInContainer(blockedListener, "nc -l 0.0.0.0:12345")
		runInContainer(unblockedListener, "nc -l 0.0.0.0:12345")
		runInContainer(allowedListener, "nc -l 0.0.0.0:12345")

		// a bit of time for the listeners to start, since they block
		time.Sleep(time.Second)

		process := runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP),
		)
		Expect(process.Wait()).To(Equal(1))

		process = runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", unblockedListenerIP),
		)
		Expect(process.Wait()).To(Equal(0))

		process = runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", allowedListenerIP),
		)
		Expect(process.Wait()).To(Equal(0))
	})
})
