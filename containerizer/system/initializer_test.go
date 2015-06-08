package system_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_container_initializer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Initializer", func() {

	var (
		step1 *fake_container_initializer.FakeContainerInitializer
		step2 *fake_container_initializer.FakeContainerInitializer
		init  *system.ContainerInitializer
	)

	BeforeEach(func() {
		step1 = &fake_container_initializer.FakeContainerInitializer{}
		step2 = &fake_container_initializer.FakeContainerInitializer{}

		init = &system.ContainerInitializer{
			Steps: []system.Initializer{step1, step2},
		}
	})

	It("runs each initializer method", func() {
		Expect(init.Init()).To(Succeed())
		Expect(step1.InitCallCount()).To(Equal(1))
		Expect(step2.InitCallCount()).To(Equal(1))
	})

	Context("when an early step fails", func() {
		BeforeEach(func() {
			step1.InitReturns(errors.New("aaaaaaa"))
		})

		It("does not run subsequent steps", func() {
			Expect(init.Init()).To(MatchError("aaaaaaa"))
			Expect(step1.InitCallCount()).To(Equal(1))
			Expect(step2.InitCallCount()).To(Equal(0))
		})
	})
})
