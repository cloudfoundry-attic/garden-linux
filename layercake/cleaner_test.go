package layercake_test

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	"github.com/docker/docker/image"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Oven cleaner", func() {
	var gc *layercake.OvenCleaner
	var fakeCake *fake_cake.FakeCake
	var fakeRetainer *fake_retainer.FakeRetainer
	var child2parent map[layercake.ID]layercake.ID // child -> parent

	BeforeEach(func() {
		fakeCake = new(fake_cake.FakeCake)
		fakeRetainer = new(fake_retainer.FakeRetainer)
		gc = &layercake.OvenCleaner{
			Cake:     fakeCake,
			Retainer: fakeRetainer,
			Logger:   lagertest.NewTestLogger("test"),
		}

		child2parent = make(map[layercake.ID]layercake.ID)
		fakeCake.GetStub = func(id layercake.ID) (*image.Image, error) {
			if parent, ok := child2parent[id]; ok {
				return &image.Image{ID: id.GraphID(), Parent: parent.GraphID()}, nil
			}

			return &image.Image{}, nil
		}

		fakeCake.IsLeafStub = func(id layercake.ID) (bool, error) {
			for _, p := range child2parent {
				if p == id {
					return false, nil
				}
			}

			return true, nil
		}

		fakeCake.RemoveStub = func(id layercake.ID) error {
			delete(child2parent, id)
			return nil
		}
	})

	Describe("Remove", func() {
		Context("when the layer has no parents", func() {
			BeforeEach(func() {
				fakeCake.GetReturns(&image.Image{}, nil)
			})

			It("removes the layer", func() {
				Expect(gc.Remove(layercake.ContainerID("child"))).To(Succeed())
				Expect(fakeCake.RemoveCallCount()).To(Equal(1))
				Expect(fakeCake.RemoveArgsForCall(0)).To(Equal(layercake.ContainerID("child")))
			})

			Context("when the layer is retained", func() {
				BeforeEach(func() {
					fakeRetainer.IsHeldReturns(true)
				})

				It("should not remove the layer", func() {
					Expect(gc.Remove(layercake.ContainerID("child"))).To(Succeed())
					Expect(fakeCake.RemoveCallCount()).To(Equal(0))
				})
			})
		})

		Context("when removing fails", func() {
			It("returns an error", func() {
				fakeCake.RemoveReturns(errors.New("cake failure"))
				Expect(gc.Remove(layercake.ContainerID("whatever"))).To(MatchError("cake failure"))
			})
		})

		Context("when the layer has a parent", func() {
			BeforeEach(func() {
				child2parent[layercake.ContainerID("child")] = layercake.DockerImageID("parent")
			})

			Context("and the parent has no other children", func() {
				It("removes the layer, and its parent", func() {
					Expect(gc.Remove(layercake.ContainerID("child"))).To(Succeed())

					Expect(fakeCake.RemoveCallCount()).To(Equal(2))
					Expect(fakeCake.RemoveArgsForCall(0)).To(Equal(layercake.ContainerID("child")))
					Expect(fakeCake.RemoveArgsForCall(1)).To(Equal(layercake.DockerImageID("parent")))
				})
			})

			Context("when removing fails", func() {
				It("does not remove any more layers", func() {
					fakeCake.RemoveReturns(errors.New("cake failure"))
					gc.Remove(layercake.ContainerID("whatever"))
					Expect(fakeCake.RemoveCallCount()).To(Equal(1))
				})
			})

			Context("but the layer has another child", func() {
				BeforeEach(func() {
					child2parent[layercake.ContainerID("some-other-child")] = layercake.DockerImageID("parent")
				})

				It("removes only the initial layer", func() {
					child2parent[layercake.ContainerID("child")] = layercake.DockerImageID("parent")
					Expect(gc.Remove(layercake.ContainerID("child"))).To(Succeed())

					Expect(fakeCake.RemoveCallCount()).To(Equal(1))
					Expect(fakeCake.RemoveArgsForCall(0)).To(Equal(layercake.ContainerID("child")))
				})
			})
		})

		Context("when the layer has grandparents", func() {
			It("removes all the grandparents", func() {
				child2parent[layercake.ContainerID("child")] = layercake.DockerImageID("parent")
				child2parent[layercake.DockerImageID("parent")] = layercake.DockerImageID("granddaddy")

				Expect(gc.Remove(layercake.ContainerID("child"))).To(Succeed())

				Expect(fakeCake).To(HaveBeenCalledWith("Remove", layercake.ContainerID("child")))
				Expect(fakeCake).To(HaveBeenCalledWith("Path", layercake.DockerImageID("parent")))
				Expect(fakeCake).To(HaveBeenCalledWith("Remove", layercake.DockerImageID("granddaddy")))
			})
		})
	})
})

type hbcw struct {
	expectedMethod string
	expectedArgs   []interface{}

	msg, negatedMsg string
}

func HaveBeenCalledWith(method string, args ...interface{}) *hbcw {
	return &hbcw{expectedMethod: method, expectedArgs: args}
}

func (h *hbcw) Match(actual interface{}) (success bool, err error) {
	cc := reflect.ValueOf(actual).MethodByName(h.expectedMethod + "CallCount").Call(nil)[0].Int()

	if cc == 0 {
		h.msg = fmt.Sprintf("Expected %s method to have been called on %#v, but it was not", h.expectedMethod, actual)
		return false, nil
	}

INVOCATIONS:
	for i := 0; i < int(cc); i++ {
		m := reflect.ValueOf(actual).MethodByName(h.expectedMethod + "ArgsForCall")
		result := m.Call([]reflect.Value{reflect.ValueOf(i)})

		for j := 0; j < len(result); j++ {
			if result[j].Interface() != h.expectedArgs[j] {
				continue INVOCATIONS
			}
		}

		return true, nil
	}

	h.msg = fmt.Sprintf("Expected method %s to be called with args %s", h.expectedMethod, h.expectedArgs)
	return false, nil
}

func (h *hbcw) FailureMessage(actual interface{}) (message string) {
	return h.msg
}

func (h *hbcw) NegatedFailureMessage(actual interface{}) (message string) {
	return h.negatedMsg
}
