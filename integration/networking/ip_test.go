package networking_test

import (
	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"os/exec"
)

var _ = Describe("IP settings", func() {
	var container api.Container

	BeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(api.ContainerSpec{Network: "10.3.0.0/24"})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("container's network interface", func() {
		It("has the correct IP address", func() {
			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()

			process, err := container.Run(api.ProcessSpec{
				Path: "/sbin/ifconfig",
				Args: []string{containerIfName(container)},
			}, api.ProcessIO{
				Stdout: stdout,
				Stderr: stderr,
			})
			Ω(err).ShouldNot(HaveOccurred())

			rc, err := process.Wait()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(rc).Should(Equal(0))

			Ω(stdout.Contents()).Should(ContainSubstring(" inet addr:10.3.0.1 "))
		})
	})

	Describe("hosts's network interface for a container", func() {
		It("has the correct IP address", func() {
			out, err := exec.Command("/sbin/ifconfig", hostIfName(container)).Output()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(out).Should(ContainSubstring(" inet addr:10.3.0.254 "))
		})
	})

	Describe("the container's network", func() {
		It("is reachable from the host", func() {
			info, ierr := container.Info()
			Ω(ierr).ShouldNot(HaveOccurred())

			out, err := exec.Command("/bin/ping", "-c 2", info.ContainerIP).Output()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(out).Should(ContainSubstring("0% packet loss"))
		})
	})

	Describe("the internet", func() {
		It("is reachable from inside the container", func() {
			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()

			process, err := container.Run(api.ProcessSpec{
				Path: "/bin/ping",
				Args: []string{"-c", "2", "8.8.8.8"},
			}, api.ProcessIO{
				Stdout: stdout,
				Stderr: stderr,
			})
			Ω(err).ShouldNot(HaveOccurred())

			rc, err := process.Wait()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(rc).Should(Equal(0))

			Ω(stdout.Contents()).Should(ContainSubstring("0% packet loss"))
		})
	})

})
