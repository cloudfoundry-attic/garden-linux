package bridgemgr_test

import (
	"code.cloudfoundry.org/garden-linux/network/bridgemgr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridge Name Generator", func() {
	var (
		generator bridgemgr.BridgeNameGenerator
	)

	BeforeEach(func() {
		generator = bridgemgr.NewBridgeNameGenerator("pr-")
	})

	It("returns unique names each time it is called", func() {
		generatedNames := make(map[string]bool)

		for i := 0; i < 100; i++ {
			name := generator.Generate()
			generatedNames[name] = true
		}

		Expect(generatedNames).To(HaveLen(100))
	})

	It("includes the entire prefix as part of the name", func() {
		name := generator.Generate()
		Expect(name).To(HavePrefix("pr-"))
	})

	It("returns names that are exactly 15 characters", func() {
		name := generator.Generate()
		Expect(name).To(HaveLen(15))

		name = bridgemgr.NewBridgeNameGenerator("p").Generate()
		Expect(name).To(HaveLen(15))
	})
})
