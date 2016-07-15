package hook_test

import (
	"code.cloudfoundry.org/garden-linux/hook"

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
			Expect(wasRun).To(BeTrue())
		})
	})

	Context("when the first argument does not name a registered hook", func() {
		It("panics", func() {
			Expect(func() { registry.Main("does-not-hook") }).To(Panic())
		})
	})

	Context("when multiple hooks are registered with the same name", func() {
		It("panics", func() {
			registry.Register("a-hook", func() {})
			Expect(func() { registry.Register("a-hook", func() {}) }).To(Panic())
		})
	})
})
