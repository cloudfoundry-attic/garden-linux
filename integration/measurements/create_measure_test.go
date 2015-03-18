package measurements_test

import (
	"runtime"
	"strconv"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	createDestroys = 0 // e.g. 10
	createSamples  = 0 // e.g. 5
)

var _ = Describe("Container creation", func() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var (
		createDestroy   func(i int, b Benchmarker)
		goCreateDestroy func(i int, b Benchmarker) chan struct{}
	)

	BeforeEach(func() {
		client = startGarden()

		createDestroy = func(i int, b Benchmarker) {
			b.Time("total create+destroy", func() {
				b.Time("create+destroy-"+strconv.Itoa(i), func() {
					var ctr garden.Container
					b.Time("total create", func() {
						b.Time("create-"+strconv.Itoa(i), func() {
							var err error
							ctr, err = client.Create(garden.ContainerSpec{})
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
					b.Time("total destroy", func() {
						b.Time("destroy-"+strconv.Itoa(i), func() {
							err := client.Destroy(ctr.Handle())
							Ω(err).ShouldNot(HaveOccurred())
						})
					})
				})
			})
		}

		goCreateDestroy = func(i int, b Benchmarker) chan struct{} {
			done := make(chan struct{})
			go func() {
				createDestroy(i, b)
				close(done)
			}()
			return done
		}
	})

	Measure("multiple creates and destroys", func(b Benchmarker) {
		b.Time("create+destroy concurrently "+strconv.Itoa(createDestroys)+" times", func() {
			chans := make([]chan struct{}, createDestroys)

			for i, _ := range chans {
				chans[i] = goCreateDestroy(i, b)
			}

			for i, _ := range chans {
				Eventually(chans[i], "10s").Should(BeClosed())
			}
		})
	}, createSamples)

})
