package lifecycle_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Security", func() {
	Describe("Isolating PIDs", func() {
		It("isolates processes so that only process from inside the container are visible", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				Path: "sleep",
				Args: []string{"989898"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

			psout := gbytes.NewBuffer()
			_, err = container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "ps -a | tail -n +2 | wc -l"},
			}, garden.ProcessIO{
				Stdout: psout,
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(psout).Should(gbytes.Say("6")) // wshd, sleep, ps, tail, sh, wc
		})
	})

	Describe("Denying access to network ranges", func() {
		var (
			blockedListener   garden.Container
			blockedListenerIP string = fmt.Sprintf("11.0.%d.1", GinkgoParallelNode())

			unblockedListener   garden.Container
			unblockedListenerIP string = fmt.Sprintf("11.1.%d.1", GinkgoParallelNode())

			allowedListener   garden.Container
			allowedListenerIP string = fmt.Sprintf("11.2.%d.1", GinkgoParallelNode())

			sender garden.Container
		)

		BeforeEach(func() {
			client = startGarden(
				"-denyNetworks", strings.Join([]string{
					blockedListenerIP + "/32",
					allowedListenerIP + "/32",
				}, ","),
				"-allowNetworks", allowedListenerIP+"/32",
			)

			var err error

			// create a listener to which we deny network access
			blockedListener, err = client.Create(garden.ContainerSpec{Network: blockedListenerIP + "/30"})
			Ω(err).ShouldNot(HaveOccurred())
			blockedListenerIP = containerIP(blockedListener)

			// create a listener to which we do not deny access
			unblockedListener, err = client.Create(garden.ContainerSpec{Network: unblockedListenerIP + "/30"})
			Ω(err).ShouldNot(HaveOccurred())
			unblockedListenerIP = containerIP(unblockedListener)

			// create a listener to which we exclicitly allow access
			allowedListener, err = client.Create(garden.ContainerSpec{Network: allowedListenerIP + "/30"})
			Ω(err).ShouldNot(HaveOccurred())
			allowedListenerIP = containerIP(allowedListener)

			// create a container with the new deny network configuration
			sender, err = client.Create(garden.ContainerSpec{})
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

		runInContainer := func(container garden.Container, script string) garden.Process {
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", script},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Ω(err).ShouldNot(HaveOccurred())

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
})
