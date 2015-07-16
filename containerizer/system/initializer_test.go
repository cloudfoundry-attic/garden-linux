package system_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_step_runner"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Initializer", func() {

	var (
		step1 *fake_step_runner.FakeStepRunner
		step2 *fake_step_runner.FakeStepRunner
		init  *system.Initializer
	)

	BeforeEach(func() {
		step1 = &fake_step_runner.FakeStepRunner{}
		step2 = &fake_step_runner.FakeStepRunner{}

		init = &system.Initializer{
			Steps: []system.StepRunner{step1, step2},
		}
	})

	It("runs each initializer method", func() {
		Expect(init.Init()).To(Succeed())
		Expect(step1.RunCallCount()).To(Equal(1))
		Expect(step2.RunCallCount()).To(Equal(1))
	})

	Context("when an early step fails", func() {
		BeforeEach(func() {
			step1.RunReturns(errors.New("aaaaaaa"))
		})

		It("does not run subsequent steps", func() {
			Expect(init.Init()).To(MatchError("aaaaaaa"))
			Expect(step1.RunCallCount()).To(Equal(1))
			Expect(step2.RunCallCount()).To(Equal(0))
		})
	})
})
