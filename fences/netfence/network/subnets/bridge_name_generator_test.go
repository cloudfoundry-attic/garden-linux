package subnets_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridge Name Generator", func() {
	var (
		generator subnets.BridgeNameGenerator
	)

	BeforeEach(func() {
		generator = subnets.NewBridgeNameGenerator("prefix")
	})

	It("returns unique names each time it is called", func() {
		generatedNames := make(map[string]bool)

		for i := 0; i < 100; i++ {
			name := generator.Generate()
			generatedNames[name] = true
		}

		Ω(generatedNames).Should(HaveLen(100))
	})

	It("includes the truncated prefix and b- at the start of the name", func() {
		name := generator.Generate()

		Ω(name).Should(HavePrefix("prb-"))
	})

	It("allows single character prefixes", func() {
		generator = subnets.NewBridgeNameGenerator("p")
		name := generator.Generate()

		Ω(name).Should(HavePrefix("pb-"))
	})

	It("returns names that are exactly 15 bytes", func() {
		name := generator.Generate()

		Ω(name).Should(HaveLen(15))
	})
})
