package networking_test

import (
	"os/exec"
	"strconv"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("IP settings", func() {
	var (
		container          api.Container
		containerNetwork   string
		containerInterface string
		hostInterface      string
	)

	JustBeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(api.ContainerSpec{Network: containerNetwork})
		Ω(err).ShouldNot(HaveOccurred())

		containerInterface = "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle() + "-1"
		hostInterface = "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle() + "-0"
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("when the Network parameter is a subnet address", func() {
		BeforeEach(func() {
			containerNetwork = "10.3.0.0/24"
		})

		Describe("container's network interface", func() {
			It("has the correct IP address", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Run(api.ProcessSpec{
					Path: "/sbin/ifconfig",
					Args: []string{containerInterface},
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

				out, err := exec.Command("/sbin/ifconfig", hostInterface).Output()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(out).Should(ContainSubstring(" inet addr:10.3.0.254 "))
			})
		})
	})

	Context("when the Network parameter is not a subnet address", func() {
		BeforeEach(func() {
			containerNetwork = "10.3.0.2/24"
		})

		Describe("container's network interface", func() {
			It("has the specified IP address", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Run(api.ProcessSpec{
					Path: "/sbin/ifconfig",
					Args: []string{containerInterface},
				}, api.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Ω(err).ShouldNot(HaveOccurred())
				rc, err := process.Wait()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(rc).Should(Equal(0))

				Ω(stdout.Contents()).Should(ContainSubstring(" inet addr:10.3.0.2 "))
			})
		})

		Describe("hosts's network interface for a container", func() {
			It("has the correct IP address", func() {

				out, err := exec.Command("/sbin/ifconfig", hostInterface).Output()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(out).Should(ContainSubstring(" inet addr:10.3.0.254 "))
			})
		})
	})

})
