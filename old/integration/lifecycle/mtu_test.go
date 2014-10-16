package lifecycle_test

import (
	"os/exec"
	"strconv"

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

			containerInterface := "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle() + "-1"

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

			Ω(stdout.Contents()).Should(ContainSubstring(" MTU:6789 "))
		})
	})

	Describe("hosts's network interface for a container", func() {
		It("has the correct MTU size", func() {
			hostInterface := "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle() + "-0"

			out, err := exec.Command("/sbin/ifconfig", hostInterface).Output()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(out).Should(ContainSubstring(" MTU:6789 "))
		})
	})

})
