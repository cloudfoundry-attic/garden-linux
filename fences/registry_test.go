package fences_test

import (
	"encoding/json"
	"errors"
	"flag"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fence Registry", func() {
	Describe("Registering a Fence", func() {
		It("passes a given flagset to all Init funcs", func() {
			registry := &FlagProcessors{}

			calledWithFs := make([]*flag.FlagSet, 0)
			for i := 0; i < 2; i++ {
				registry.Register(func(fs *flag.FlagSet) error {
					calledWithFs = append(calledWithFs, fs)
					return nil
				}, func(*BuilderRegistry) error {
					return nil
				})
			}

			fs := &flag.FlagSet{}
			registry.Main(fs, []string{})

			Ω(calledWithFs).Should(HaveLen(2))
			Ω(calledWithFs[0]).Should(Equal(fs))
			Ω(calledWithFs[1]).Should(Equal(fs))
		})
	})

	It("calls all main functions", func() {
		registry := &FlagProcessors{}

		mainCalled := 0
		for i := 0; i < 2; i++ {
			registry.Register(func(fs *flag.FlagSet) error {
				return nil
			}, func(*BuilderRegistry) error {
				mainCalled++
				return nil
			})
		}

		fs := &flag.FlagSet{}
		registry.Main(fs, []string{})

		Ω(mainCalled).Should(Equal(2))
	})

	It("returns an error if any of the main functions error", func() {

		registry := &FlagProcessors{}

		mainCalled := 0
		for i := 0; i < 2; i++ {
			registry.Register(func(fs *flag.FlagSet) error {
				return nil
			}, func(*BuilderRegistry) error {
				if mainCalled > 0 {
					return errors.New("o no")
				}

				mainCalled++
				return nil
			})
		}

		fs := &flag.FlagSet{}
		_, err := registry.Main(fs, []string{})

		Ω(err).Should(MatchError("o no"))
		Ω(mainCalled).Should(Equal(1))
	})

	It("parses the flagset before calling all main funcs", func() {
		registry := &FlagProcessors{}

		var flag1 string
		registry.Register(func(fs *flag.FlagSet) error {
			fs.StringVar(&flag1, "flag1", "desc", "no")
			return nil
		}, func(*BuilderRegistry) error {
			Ω(flag1).Should(Equal("1"))
			return nil
		})

		var flag2 string
		registry.Register(func(fs *flag.FlagSet) error {
			fs.StringVar(&flag2, "flag2", "desc", "no")
			return nil
		}, func(*BuilderRegistry) error {
			Ω(flag2).Should(Equal("2"))
			return nil
		})

		fs := &flag.FlagSet{}
		registry.Main(fs, []string{"-flag1", "1", "-flag2", "2"})
	})

	Describe("Build", func() {
		Context("when no Allocators are registered", func() {
			It("returns ErrNoFencesRegistered", func() {
				r := &BuilderRegistry{}
				_, err := r.Build("", nil, "")
				Ω(err).Should(Equal(ErrNoFencesRegistered))
			})
		})

		Context("when an Allocator is registered", func() {
			var (
				registry      *BuilderRegistry
				fakeAllocator *FakeAllocator
			)

			BeforeEach(func() {
				r := &FlagProcessors{}
				fakeAllocator = &FakeAllocator{}
				r.Register(func(fs *flag.FlagSet) error { return nil }, func(r *BuilderRegistry) error {
					r.Register(fakeAllocator)
					return nil
				})

				var err error
				registry, err = r.Main(&flag.FlagSet{}, []string{})
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("and when it returns an error", func() {
				It("returns the error to the caller", func() {
					fakeAllocator.allocateError = errors.New("o no")
					_, err := registry.Build("xyz", nil, "")
					Ω(err).Should(MatchError("o no"))
				})
			})

			Context("and when it succeeds", func() {
				It("calls the registered allocator and returns the result", func() {
					fakeAllocation := &FakeAllocation{"fake"}
					fakeAllocator.allocate = fakeAllocation

					allocation, err := registry.Build("xyz", nil, "")
					Ω(err).ShouldNot(HaveOccurred())
					Ω(allocation).Should(Equal(fakeAllocation))
				})
			})
		})

		Context("when multiple allocators are registered", func() {
			It("defensively panics, since this isn't implemented yet", func() {
				r := &BuilderRegistry{}
				r.Register(&FakeAllocator{})
				Ω(func() { r.Register(&FakeAllocator{}) }).Should(Panic())
			})
		})
	})

	Describe("Recover", func() {
		Context("when no Allocators are registered", func() {
			It("returns ErrNoFencesRegistered", func() {
				r := &BuilderRegistry{}
				_, err := r.Rebuild(nil)
				Ω(err).Should(Equal(ErrNoFencesRegistered))
			})
		})

		Context("when an Allocator is registered", func() {
			var (
				registry      *BuilderRegistry
				fakeAllocator *FakeAllocator
			)

			BeforeEach(func() {
				r := &FlagProcessors{}
				fakeAllocator = &FakeAllocator{}
				r.Register(func(fs *flag.FlagSet) error { return nil }, func(r *BuilderRegistry) error {
					r.Register(fakeAllocator)
					return nil
				})

				var err error
				registry, err = r.Main(&flag.FlagSet{}, []string{})
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("and when it returns an error", func() {
				It("returns the error to the caller", func() {
					fakeAllocator.recoverError = errors.New("o no")
					_, err := registry.Rebuild(nil)
					Ω(err).Should(MatchError("o no"))
				})
			})

			Context("and when it succeeds", func() {
				It("calls the registered allocator with the encoded data and returns the result", func() {
					fakeAllocation := &FakeAllocation{"fake"}
					fakeAllocator.allocate = fakeAllocation

					var encoded json.RawMessage = []byte("encoded")

					allocation, err := registry.Rebuild(&encoded)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(allocation).Should(Equal(fakeAllocation))
					Ω(fakeAllocator.recovered).Should(Equal(encoded))
				})
			})
		})
	})

	Describe("Capacity", func() {
		Context("when no Allocators are registered", func() {
			It("returns 0", func() {
				r := &BuilderRegistry{}
				Ω(r.Capacity()).Should(Equal(0))
			})
		})

		Context("when an Allocator is registered", func() {
			var (
				registry      *BuilderRegistry
				fakeAllocator *FakeAllocator
			)

			BeforeEach(func() {
				r := &FlagProcessors{}
				fakeAllocator = &FakeAllocator{}
				r.Register(func(fs *flag.FlagSet) error { return nil }, func(r *BuilderRegistry) error {
					r.Register(fakeAllocator)
					return nil
				})

				var err error
				registry, err = r.Main(&flag.FlagSet{}, []string{})
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("calls the registered allocator and returns the result", func() {
				capacity := registry.Capacity()
				Ω(capacity).Should(Equal(fakeAllocator.capacity))
			})
		})
	})
})

type FakeAllocator struct {
	allocateError error
	recoverError  error
	allocate      Fence
	recovered     json.RawMessage
	capacity      int
}

func (f *FakeAllocator) Build(spec string, sysconfig *sysconfig.Config, containerID string) (Fence, error) {
	if f.allocateError != nil {
		return nil, f.allocateError
	}

	return f.allocate, nil
}

func (f *FakeAllocator) Rebuild(r *json.RawMessage) (Fence, error) {
	if f.recoverError != nil {
		return nil, f.recoverError
	}

	f.recovered = *r

	return f.allocate, nil
}

func (f *FakeAllocator) Capacity() int {
	return f.capacity
}

type FakeAllocation struct {
	name string
}

func (a *FakeAllocation) Deconfigure() error {
	return nil
}

func (a *FakeAllocation) Dismantle() error {
	return nil
}

func (a *FakeAllocation) Info(i *garden.ContainerInfo) {
}

func (a *FakeAllocation) MarshalJSON() ([]byte, error) {
	return nil, nil
}

func (a *FakeAllocation) ConfigureProcess(env process.Env) error {
	return nil
}

func (a *FakeAllocation) String() string {
	return "fake allocation"
}
