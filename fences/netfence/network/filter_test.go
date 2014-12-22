package network_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Filter", func() {
	It("constructs a filter factory correctly from a tag", func() {
		ff := network.NewFilterFactory("tag")
		Ω(ff.String()).Should(Equal(`&network.filterFactory{instancePrefix:"w-tag-instance-"}`))
	})

	Describe("FilterFactory", func() {
		var filterFactory network.FilterFactory
		BeforeEach(func() {
			filterFactory = network.NewFilterFactory("tag")
		})

		It("constructs a filter from a container id", func() {
			filter := filterFactory.Create("id")
			Ω(filter).ShouldNot(BeNil())
		})
	})

})
