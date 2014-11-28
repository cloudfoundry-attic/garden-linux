package lifecycle_test

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/garden/api"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("With a blocked network range", func() {
	var (
		blockedListener   api.Container
		blockedListenerIP string = fmt.Sprintf("11.0.%d.1", GinkgoParallelNode())

		sender   api.Container
		senderIP string = fmt.Sprintf("11.0.%d.2", GinkgoParallelNode())
	)

	BeforeEach(func() {
		client = startGarden(
			"-denyNetworks",
			blockedListenerIP+"/32",
		)

		var err error

		// create a listener to which we deny network access
		blockedListener, err = client.Create(api.ContainerSpec{Network: blockedListenerIP + "/24"})
		Ω(err).ShouldNot(HaveOccurred())
		blockedListenerIP = containerIP(blockedListener)

		// create a container with the new deny network configuration
		sender, err = client.Create(api.ContainerSpec{Network: senderIP + "/24"})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(sender.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		err = client.Destroy(blockedListener.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	runInContainer := func(container api.Container, script string) api.Process {
		process, err := container.Run(api.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", script},
		}, api.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})
		Ω(err).ShouldNot(HaveOccurred())

		return process
	}

	It("containers on the blocked subnet are not blocked from each other", func() {
		runInContainer(blockedListener, "nc -l 0.0.0.0:12345")

		// a bit of time for the listeners to start, since they block
		time.Sleep(5 * time.Second)

		process := runInContainer(
			sender,
			fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP),
		)
		Ω(process.Wait()).Should(Equal(0))
	})
})
