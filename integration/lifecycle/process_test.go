package lifecycle_test

import (
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Process", func() {
	var (
		gardenClient        garden.Client
		container           garden.Container
		container2          garden.Container
		rootfs              string
		privilegedContainer bool
	)

	BeforeEach(func() {
		rootfs = "docker:///busybox"
		privilegedContainer = false
	})

	JustBeforeEach(func() {
		gardenClient = startGarden()

		var err error
		container, err = gardenClient.Create(garden.ContainerSpec{
			RootFSPath: rootfs,
			Privileged: privilegedContainer,
		})
		Expect(err).ToNot(HaveOccurred())

		container2, err = gardenClient.Create(garden.ContainerSpec{
			RootFSPath: rootfs,
			Privileged: privilegedContainer,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(gardenClient.Destroy(container.Handle())).To(Succeed())

		// err := client.DestroyAndStop()
		// client.Cleanup()
		// Expect(err).NotTo(HaveOccurred())
	})

	PIt("does not leak file descriptors", func() {
		process, err := container.Run(garden.ProcessSpec{
			User: "vcap",
			Path: "/bin/sleep",
			Args: []string{"1000"},
		}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
		Expect(err).NotTo(HaveOccurred())

		_, err = process.Wait()
		Expect(err).ToNot(HaveOccurred())
		session, err := gexec.Start(exec.Command("lsof", "-n", "-c", "wsh"), GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session).Should(gbytes.Say("Whateva"))
	})

	FIt("does not leak file descriptors loop", func() {
		for index := 0; index < 1000; index++ {
			process, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/bin/ls",
				Args: []string{"1000"},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Expect(err).NotTo(HaveOccurred())

			process2, err := container2.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/bin/ls",
				Args: []string{"1000"},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Expect(err).NotTo(HaveOccurred())

			_, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			_, err = process2.Wait()
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(1 * time.Second)
			// session, err := gexec.Start(exec.Command("lsof", "-n", "-c", "wsh"), GinkgoWriter, GinkgoWriter)
			// Expect(err).NotTo(HaveOccurred())
			// Eventually(session).Should(gbytes.Say("Whateva"))
		}
	})

})
