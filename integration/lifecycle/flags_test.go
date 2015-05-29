package lifecycle_test

import (
	"fmt"
	"net/http"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Garden startup flags", func() {

	var debugAddr string

	BeforeEach(func() {
		debugAddr = fmt.Sprintf("0.0.0.0:%d", 15000+GinkgoParallelNode())
	})

	Context("when starting without the --debugAddr flag", func() {
		BeforeEach(func() {
			client = startGarden()
		})

		It("does not expose the pprof debug endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
			Expect(err).To(HaveOccurred())
		})

		It("does not expose the log level adjustment endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d debug", debugAddr))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when started with the --maxContainers flag", func() {
		Context("when maxContainers is lower than the subnet pool capacity", func() {
			BeforeEach(func() {
				client = startGarden("--maxContainers", "1")
			})

			Context("when attempting to create more than maxContainers containers", func() {
				It("returns an error", func() {
					c1, err := client.Create(garden.ContainerSpec{})
					Expect(err).NotTo(HaveOccurred())
					defer client.Destroy(c1.Handle())
					_, err = client.Create(garden.ContainerSpec{})
					Expect(err).To(MatchError(ContainSubstring("cannot create more than 1 containers")))
				})
			})

			Context("when getting the capacity", func() {
				It("returns the maxContainers flag value", func() {
					capacity, err := client.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(1)))
				})
			})
		})

		Context("when maxContainers is higher than the subnet pool capacity", func() {
			BeforeEach(func() {
				client = startGarden("--maxContainers", "1000")
			})

			Context("when getting the capacity", func() {
				It("returns the capacity of the subnet pool", func() {
					capacity, err := client.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(64)))
				})
			})
		})
	})

	Context("when starting with the --debugAddr flag", func() {
		BeforeEach(func() {
			client = startGarden("--debugAddr", debugAddr)
		})

		It("exposes the pprof debug endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
			Expect(err).ToNot(HaveOccurred())
		})

		It("exposes the log level adjustment endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d debug", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d info", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d error", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d fatal", debugAddr))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
