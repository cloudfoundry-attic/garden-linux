package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container with a grace time", func() {
	var container api.Container

	BeforeEach(func() {
		client = startGarden("--containerGraceTime", "3s")

		var err error

		container, err = client.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("when a request takes longer than the grace time", func() {
		It("is not destroyed after the request is over", func() {
			process, err := container.Run(api.ProcessSpec{
				Path: "sleep",
				Args: []string{"5"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0))

			_, err = container.Info()
			Ω(err).ShouldNot(HaveOccurred())
		})
	})

	Context("when no requests are made for longer than the grace time", func() {
		It("is destroyed", func() {
			Eventually(func() error {
				_, err := client.Lookup(container.Handle())
				return err
			}, 10, 1).Should(HaveOccurred())
		})
	})
})
