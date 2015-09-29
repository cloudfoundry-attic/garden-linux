package repository_fetcher_test

import (
	"math"
	"math/rand"

	"sync"

	"runtime"

	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Graph Lock", func() {
	var lock *repository_fetcher.FetchLock

	BeforeEach(func() {
		lock = repository_fetcher.NewFetchLock()
	})

	Describe("Acquire", func() {
		Context("when the layer is locked", func() {
			BeforeEach(func() {
				lock.Acquire("some-key")
			})

			It("waits for the lock to be released", func() {
				gotLock := make(chan struct{}, 1)

				go func(lock *repository_fetcher.FetchLock, gotLock chan struct{}) {
					go GinkgoRecover()
					lock.Acquire("some-key")
					close(gotLock)
				}(lock, gotLock)

				Consistently(gotLock, "100ms").ShouldNot(BeClosed())

				lock.Release("some-key")
				Eventually(gotLock, "100ms").Should(BeClosed())
			})
		})

		Context("when the layer is not locked", func() {
			It("does not block", func(done Done) {
				lock.Acquire("some-key")

				close(done)
			}, 1.0)
		})

		Context("with multiple requests", func() {
			keysPool := []string{"key-1", "key-2", "key-3", "key-4", "key-5", "key-6", "key-7"}

			It("does not hang", func(done Done) {
				wg := new(sync.WaitGroup)
				wg.Add(1000)

				for i := 0; i < 1000; i++ {
					keyI := int(math.Abs(rand.NormFloat64())*100) % len(keysPool)
					key := keysPool[keyI]

					go func(lock *repository_fetcher.FetchLock, key string, wg *sync.WaitGroup) {
						lock.Acquire(key)
						runtime.Gosched()
						lock.Release(key)
						wg.Done()
					}(lock, key, wg)
				}

				wg.Wait()
				close(done)
			}, 10.0)
		})
	})

	Describe("Release", func() {
		Context("when the layer is not locked", func() {
			It("returns an error", func() {
				Expect(lock.Release("some-key")).To(MatchError(ContainSubstring("repository_fetcher: releasing lock: no lock for key: some-key")))
			})
		})
	})
})
