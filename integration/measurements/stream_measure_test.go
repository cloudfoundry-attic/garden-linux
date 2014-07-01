package measurements_test

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The Warden server", func() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	BeforeEach(func() {
		client = startWarden()
	})

	Describe("streaming output from a chatty job", func() {
		var container warden.Container

		BeforeEach(func() {
			var err error

			container, err = client.Create(warden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())
		})

		streamCounts := []int{0}

		for i := 1; i <= 128; i *= 2 {
			streamCounts = append(streamCounts, i)
		}

		for _, streams := range streamCounts {
			Context(fmt.Sprintf("with %d streams", streams), func() {
				var started time.Time
				var receivedBytes uint64

				numToSpawn := streams

				BeforeEach(func() {
					atomic.StoreUint64(&receivedBytes, 0)
					started = time.Now()

					spawned := make(chan bool)

					for j := 0; j < numToSpawn; j++ {
						go func() {
							defer GinkgoRecover()
							_, results, err := container.Run(warden.ProcessSpec{
								Path: "cat",
								Args: []string{"/dev/zero"},
							})
							Ω(err).ShouldNot(HaveOccurred())

							go func(results <-chan warden.ProcessStream) {
								for {
									res, ok := <-results
									if !ok {
										break
									}

									atomic.AddUint64(&receivedBytes, uint64(len(res.Data)))
								}
							}(results)

							spawned <- true
						}()
					}

					for j := 0; j < numToSpawn; j++ {
						<-spawned
					}
				})

				AfterEach(func() {
					err := client.Destroy(container.Handle())
					Ω(err).ShouldNot(HaveOccurred())
				})

				Measure("it should not adversely affect the rest of the API", func(b Benchmarker) {
					var newContainer warden.Container

					b.Time("creating another container", func() {
						var err error

						newContainer, err = client.Create(warden.ContainerSpec{})
						Ω(err).ShouldNot(HaveOccurred())
					})

					for i := 0; i < 10; i++ {
						b.Time("getting container info (10x)", func() {
							_, err := newContainer.Info()
							Ω(err).ShouldNot(HaveOccurred())
						})
					}

					for i := 0; i < 10; i++ {
						b.Time("running a job (10x)", func() {
							_, stream, err := newContainer.Run(warden.ProcessSpec{Path: "ls"})
							Ω(err).ShouldNot(HaveOccurred())

							for _ = range stream {

							}
						})
					}

					b.Time("destroying the container", func() {
						err := client.Destroy(newContainer.Handle())
						Ω(err).ShouldNot(HaveOccurred())
					})

					b.RecordValue(
						"received rate (bytes/second)",
						float64(atomic.LoadUint64(&receivedBytes))/float64(time.Since(started)/time.Second),
					)

					fmt.Println("total time:", time.Since(started))
				}, 5)
			})
		}
	})
})
