package lifecycle_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Denying access to network ranges", func() {
	var (
		blockedListener   warden.Container
		blockedListenerIP string

		unblockedListener   warden.Container
		unblockedListenerIP string

		allowedListener   warden.Container
		allowedListenerIP string

		sender warden.Container
	)

	BeforeEach(func() {
		client = startWarden()

		var err error

		// create a listener to which we deny network access
		blockedListener, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
		info, err := blockedListener.Info()
		Ω(err).ShouldNot(HaveOccurred())
		blockedListenerIP = info.ContainerIP

		// create a listener to which we do not deny access
		unblockedListener, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
		info, err = unblockedListener.Info()
		Ω(err).ShouldNot(HaveOccurred())
		unblockedListenerIP = info.ContainerIP

		// create a listener to which we exclicitly allow access
		allowedListener, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
		info, err = allowedListener.Info()
		Ω(err).ShouldNot(HaveOccurred())
		allowedListenerIP = info.ContainerIP

		restartWarden(
			"-denyNetworks", strings.Join([]string{
				blockedListenerIP + "/32",
				allowedListenerIP + "/32",
			}, ","),
			"-allowNetworks", allowedListenerIP+"/32",
		)

		// create a container with the new deny network configuration
		sender, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(sender.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		err = client.Destroy(blockedListener.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		err = client.Destroy(unblockedListener.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		err = client.Destroy(allowedListener.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	runInContainer := func(container warden.Container, script string) warden.Process {
		process, err := container.Run(warden.ProcessSpec{
			Path: "bash",
			Args: []string{"-c", script},
		}, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		return process
	}

	It("makes that block of ip addresses inaccessible to the container", func() {
		runInContainer(blockedListener, "nc -l 12345")
		runInContainer(unblockedListener, "nc -l 12345")
		runInContainer(allowedListener, "nc -l 12345")

		// a bit of time for the listeners to start, since they block
		time.Sleep(time.Second)

		process := runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP),
		)
		Ω(process.Wait()).Should(Equal(1))

		process = runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", unblockedListenerIP),
		)
		Ω(process.Wait()).Should(Equal(0))

		process = runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", allowedListenerIP),
		)
		Ω(process.Wait()).Should(Equal(0))
	})
})
