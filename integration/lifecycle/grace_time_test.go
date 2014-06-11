package lifecycle_test

import (
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container with a grace time", func() {
	var container warden.Container

	BeforeEach(func() {
		client = startWarden("--containerGraceTime", "5s")

		var err error

		container, err = client.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when a request takes longer than the grace time", func() {
		It("is not destroyed after the request is over", func() {
			_, _, err := container.Run(warden.ProcessSpec{Script: "sleep 6"})
			Expect(err).ToNot(HaveOccurred())

			_, err = container.Info()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when no requests are made for longer than the grace time", func() {
		It("is destroyed", func() {
			time.Sleep(6 * time.Second)

			_, err := container.Info()
			Expect(err).To(HaveOccurred())
		})
	})
})
