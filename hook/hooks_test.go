package hook_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/hook"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HookSet", func() {
	var registry hook.HookSet

	BeforeEach(func() {
		registry = make(hook.HookSet)
	})

	Context("when the first argument names a registered hook", func() {
		It("runs the hook", func() {
			wasRun := false
			registry.Register("a-hook", func() {
				wasRun = true
			})

			registry.Main("a-hook")
			Ω(wasRun).Should(BeTrue())
		})
	})

	Context("when the first argument does not name a registered hook", func() {
		It("panics", func() {
			Ω(func() { registry.Main("does-not-hook") }).Should(Panic())
		})
	})

	Context("when multiple hooks are registered with the same name", func() {
		It("panics", func() {
			registry.Register("a-hook", func() {})
			Ω(func() { registry.Register("a-hook", func() {}) }).Should(Panic())
		})
	})
})
