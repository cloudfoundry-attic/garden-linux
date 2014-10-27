package networking_test

import (
	"os/exec"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("MTU size", func() {
	var container api.Container

	BeforeEach(func() {
		client = startGarden("-mtu=6789")

		var err error

		container, err = client.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("container's network interface", func() {
		It("has the correct MTU size", func() {
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

			Ω(stdout.Contents()).Should(ContainSubstring(" MTU:6789 "))
		})
	})

	Describe("hosts's network interface for a container", func() {
		It("has the correct MTU size", func() {
			out, err := exec.Command("/sbin/ifconfig", hostIfName(container)).Output()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(out).Should(ContainSubstring(" MTU:6789 "))
		})
	})

})
