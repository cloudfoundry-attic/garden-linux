package network_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/devices/fakedevices"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Deconfigurer", func() {
	Describe("DeconfigureBridge", func() {
		var (
			fakeLink          *fakedevices.FakeLink
			fakeBridgeDeleter *fakedevices.FakeBridge
			log               *lagertest.TestLogger

			deconfigurer *network.Deconfigurer
		)

		BeforeEach(func() {
			log = lagertest.NewTestLogger("deconfigure")
			fakeLink = &fakedevices.FakeLink{}
			fakeBridgeDeleter = &fakedevices.FakeBridge{}

			deconfigurer = &network.Deconfigurer{
				Finder:        fakeLink,
				BridgeDeleter: fakeBridgeDeleter,
			}
		})

		Context("when the bridge device cannot be found", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return nil, false, nil
				}
			})

			It("returns success (we assume the bridge was already cleaned up)", func() {
				Ω(deconfigurer.DeconfigureBridge(log, "bar")).Should(Succeed())
			})
		})

		Context("when looking up the bridge interface fails", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return nil, true, errors.New("o no")
				}
			})

			It("returns an appropriate error", func() {
				err := deconfigurer.DeconfigureBridge(log, "bar")
				Ω(err).Should(MatchError(&network.DeleteLinkError{Cause: errors.New("o no"), Role: "bridge", Name: "bar"}))
			})
		})

		Context("when the bridge exists", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}
			})

			It("destroys the bridge", func() {
				Ω(deconfigurer.DeconfigureBridge(log, "thebridge")).Should(Succeed())
				Ω(fakeBridgeDeleter.DeleteCalledWith).Should(ContainElement(&net.Interface{Name: "thebridge"}))
			})
		})

		Context("when destroying the bridge fails", func() {
			BeforeEach(func() {
				// link exists
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}

				// deleting it fails
				fakeBridgeDeleter.DeleteReturns = errors.New("ono")
			})

			It("returns an wrapped error", func() {
				err := deconfigurer.DeconfigureBridge(log, "thebridge")
				Ω(err).Should(MatchError(&network.DeleteLinkError{Cause: errors.New("ono"), Role: "bridge", Name: "thebridge"}))
			})
		})
	})
})
