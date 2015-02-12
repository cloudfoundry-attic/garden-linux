package measurements_test

import (
	"runtime"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Container creation", func() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var (
		createDestroy   func(b Benchmarker)
		goCreateDestroy func(b Benchmarker) chan struct{}
	)

	BeforeEach(func() {
		client = startGarden()

		createDestroy = func(b Benchmarker) {
			var ctr garden.Container
			b.Time("create", func() {
				var err error
				ctr, err = client.Create(garden.ContainerSpec{})
				Ω(err).ShouldNot(HaveOccurred())
			})
			b.Time("destroy", func() {
				err := client.Destroy(ctr.Handle())
				Ω(err).ShouldNot(HaveOccurred())
			})
		}

		goCreateDestroy = func(b Benchmarker) chan struct{} {
			done := make(chan struct{})
			go func() {
				createDestroy(b)
				close(done)
			}()
			return done
		}
	})

	Measure("multiple creates and destroys", func(b Benchmarker) {
		b.Time("create and destroy", func() {

			chans := make([]chan struct{}, 10)

			for i, _ := range chans {
				chans[i] = goCreateDestroy(b)
			}

			for i, _ := range chans {
				Eventually(chans[i], "10s").Should(BeClosed())
			}
		})
	}, 3)

})
