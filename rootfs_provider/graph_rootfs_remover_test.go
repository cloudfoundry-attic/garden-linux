package rootfs_provider_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_graph"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_graph_driver"
	"github.com/docker/docker/image"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("GraphRemover", func() {
	var fakeGraphDriver *fake_graph_driver.FakeGraphDriver
	var remover *rootfs_provider.GraphCleaner
	var logger *lagertest.TestLogger
	var fakeGraph *fake_graph.FakeGraph

	var parents map[string]string

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("GraphCleaner")
		parents = make(map[string]string)

		fakeGraphDriver = new(fake_graph_driver.FakeGraphDriver)
		fakeGraphDriver.RemoveStub = func(id string) error {
			delete(parents, id)
			return nil
		}

		fakeGraph = new(fake_graph.FakeGraph)
		fakeGraph.GetStub = func(id string) (*image.Image, error) {
			return &image.Image{
				Parent: parents[id],
			}, nil
		}

		fakeGraph.IsParentStub = func(id string) bool {
			for _, value := range parents {
				if value == id {
					return true
				}
			}
			return false
		}

		remover = &rootfs_provider.GraphCleaner{
			Graph:       fakeGraph,
			GraphDriver: fakeGraphDriver,
		}
	})

	It("removes the container from the rootfs graph", func() {
		err := remover.Clean(logger, "some-id")
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeGraphDriver.PutCallCount()).To(Equal(1))
		putted := fakeGraphDriver.PutArgsForCall(0)
		Expect(putted).To(Equal("some-id"))

		Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(1))
		removed := fakeGraphDriver.RemoveArgsForCall(0)
		Expect(removed).To(Equal("some-id"))
	})

	Context("when there is a parent layer", func() {
		BeforeEach(func() {
			parents["some-id"] = "my-parent"
			parents["my-parent"] = ""
		})

		It("should delete it", func() {
			remover.Clean(logger, "some-id")

			Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(2))
			removed := fakeGraphDriver.RemoveArgsForCall(0)
			Expect(removed).To(Equal("some-id"))

			removed = fakeGraphDriver.RemoveArgsForCall(1)
			Expect(removed).To(Equal("my-parent"))
		})

		Context("when it has another child", func() {
			BeforeEach(func() {
				parents["some-id"] = "my-parent"
				parents["another-id"] = "my-parent"
			})

			It("should not delete it", func() {
				remover.Clean(logger, "some-id")

				Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(1))
				removed := fakeGraphDriver.RemoveArgsForCall(0)
				Expect(removed).To(Equal("some-id"))

				_, ok := parents["my-parent"]
				Expect(ok).To(BeTrue())

				_, ok = parents["another-id"]
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("when getting a layer fails", func() {
		BeforeEach(func() {
			fakeGraph.GetStub = func(id string) (*image.Image, error) {
				return nil, errors.New("oh no!")
			}
		})

		It("returns a error", func() {
			Expect(remover.Clean(logger, "some-id")).To(MatchError("clean graph: oh no!"))
			Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(0))
		})
	})

	Context("when removing layer fails", func() {
		BeforeEach(func() {
			fakeGraphDriver.RemoveStub = func(id string) error {
				return errors.New("oh no!")
			}
		})

		It("returns a error", func() {
			Expect(remover.Clean(logger, "some-id")).To(MatchError("clean graph: oh no!"))
			Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(1))
		})
	})

	Context("when there are multiple parents", func() {
		BeforeEach(func() {
			parents["some-id"] = "my-parent"
			parents["my-parent"] = "grandaddy"
			parents["grandaddy"] = ""
		})

		It("should delete them all", func() {
			remover.Clean(logger, "some-id")

			Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(3))
			removed := fakeGraphDriver.RemoveArgsForCall(0)
			Expect(removed).To(Equal("some-id"))

			removed = fakeGraphDriver.RemoveArgsForCall(1)
			Expect(removed).To(Equal("my-parent"))

			removed = fakeGraphDriver.RemoveArgsForCall(2)
			Expect(removed).To(Equal("grandaddy"))
		})

		Context("when grand layer has a child", func() {
			BeforeEach(func() {
				parents["baby-layer"] = "grandaddy"
			})

			It("should not delete it", func() {
				remover.Clean(logger, "some-id")

				Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(2))
				removed := fakeGraphDriver.RemoveArgsForCall(0)
				Expect(removed).To(Equal("some-id"))

				removed = fakeGraphDriver.RemoveArgsForCall(1)
				Expect(removed).To(Equal("my-parent"))

				_, ok := parents["grandaddy"]
				Expect(ok).To(BeTrue())

				_, ok = parents["baby-layer"]
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("when removing the container from the graph fails", func() {
		JustBeforeEach(func() {
			fakeGraphDriver.RemoveReturns(errors.New("oh no!"))
		})

		It("returns the error", func() {
			Expect(remover.Clean(logger, "oi")).To(MatchError("oh no!"))
		})
	})
})
