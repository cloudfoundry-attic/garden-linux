package container_repository_test

import (
	"fmt"
	"sync"

	"code.cloudfoundry.org/garden-linux/container_repository"
	"code.cloudfoundry.org/garden-linux/linux_backend"
	"code.cloudfoundry.org/garden-linux/linux_backend/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("InMemoryContainerRepository", func() {
	var containerRepo *container_repository.InMemoryContainerRepository

	BeforeEach(func() {
		containerRepo = container_repository.New()
	})

	Context("when adding and querying concurrently", func() {
		It("should not deadlock", func(done Done) {
			wg := sync.WaitGroup{}
			wg.Add(4)

			go func() {
				defer GinkgoRecover()
				defer wg.Done()

				for i := 0; i < 100; i++ {
					containerRepo.Add(fakeContainer(fmt.Sprintf("handle-%d", i)))
				}
			}()

			go func() {
				defer GinkgoRecover()
				defer wg.Done()

				for i := 0; i < 20; i++ {
					containerRepo.Delete(fakeContainer(fmt.Sprintf("handle-%d", i)))
				}
			}()

			go func() {
				defer GinkgoRecover()
				defer wg.Done()

				for i := 0; i < 50; i++ {
					containerRepo.FindByHandle(fmt.Sprintf("handle-%d", i))
				}
			}()

			go func() {
				defer GinkgoRecover()
				defer wg.Done()

				for i := 0; i < 10; i++ {
					Eventually(containerRepo.All).ShouldNot(BeEmpty())
				}
			}()

			wg.Wait()
			close(done)
		}, 10.0)
	})
})

func fakeContainer(handle string) linux_backend.Container {
	container := new(fakes.FakeContainer)
	container.HandleReturns(handle)

	return container
}
