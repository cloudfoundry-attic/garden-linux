package containerizer_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FuncStep", func() {
	var (
		counter      int
		callbackFunc func() error
		step         *containerizer.FuncStep
	)

	BeforeEach(func() {
		callbackFunc = func() error {
			counter++
			return nil
		}
	})

	JustBeforeEach(func() {
		step = &containerizer.FuncStep{callbackFunc}
	})

	It("calls the provided func", func() {
		Expect(step.Run()).Should(Succeed())
		Expect(counter).To(Equal(1))
	})

	Context("when the func fails", func() {
		BeforeEach(func() {
			callbackFunc = func() error {
				return errors.New("banana")
			}
		})

		It("returns the error", func() {
			Expect(step.Run()).To(MatchError("banana"))
		})
	})

	Context("when the func is not defined", func() {
		BeforeEach(func() {
			callbackFunc = nil
		})

		It("returns an error", func() {
			Expect(step.Run()).To(MatchError("containerizer: callback function is not defined"))
		})
	})
})
