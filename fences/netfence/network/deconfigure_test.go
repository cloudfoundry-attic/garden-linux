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
	Describe("DeconfigureHost", func() {
		var (
			fakeLink          *fakedevices.FakeLink
			fakeBridgeDeleter *fakedevices.FakeBridge
			fakeHostDeleter   *fakedevices.FakeLink
			log               *lagertest.TestLogger

			deconfigurer *network.Deconfigurer
		)

		BeforeEach(func() {
			log = lagertest.NewTestLogger("deconfigure")
			fakeLink = &fakedevices.FakeLink{}
			fakeHostDeleter = &fakedevices.FakeLink{}
			fakeBridgeDeleter = &fakedevices.FakeBridge{}

			deconfigurer = &network.Deconfigurer{
				Finder:        fakeLink,
				HostDeleter:   fakeHostDeleter,
				BridgeDeleter: fakeBridgeDeleter,
			}
		})

		Context("when the host device exists", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					return &net.Interface{Name: name}, true, nil
				}
			})

			It("destroys it", func() {
				Ω(deconfigurer.DeconfigureHost(log, "foo", "bar")).Should(Succeed())
				Ω(fakeHostDeleter.DeleteCalledWith).Should(ContainElement(&net.Interface{Name: "foo"}))
			})

			Context("when destroying the host interface fails", func() {
				BeforeEach(func() {
					fakeHostDeleter.DeleteReturns = errors.New("ono")
				})

				It("returns an wrapped error", func() {
					err := deconfigurer.DeconfigureHost(log, "foo", "bar")
					Ω(err).Should(MatchError(&network.DeleteLinkError{Cause: errors.New("ono"), Role: "host", Name: "foo"}))
				})

				It("does not attempt to destroy the bridge", func() {
					fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
						return &net.Interface{Name: name}, true, nil
					}

					Ω(deconfigurer.DeconfigureHost(log, "thehost", "thebridge")).ShouldNot(Succeed())
					Ω(fakeBridgeDeleter.DeleteCalledWith).Should(BeEmpty())
				})
			})

			It("destroys the bridge", func() {
				Ω(deconfigurer.DeconfigureHost(log, "thehost", "thebridge")).Should(Succeed())
				Ω(fakeBridgeDeleter.DeleteCalledWith).Should(ContainElement(&net.Interface{Name: "thebridge"}))
			})

			Context("when the bridge device cannot be found", func() {
				BeforeEach(func() {
					fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
						if name == "thehost" {
							return nil, false, nil
						}

						return &net.Interface{Name: name}, true, nil
					}
				})

				It("returns success (we assume the bridge was already cleaned up)", func() {
					Ω(deconfigurer.DeconfigureHost(log, "foo", "bar")).Should(Succeed())
				})
			})

			Context("when destroying the bridge fails", func() {
				BeforeEach(func() {
					fakeBridgeDeleter.DeleteReturns = errors.New("ono")
				})

				It("returns an wrapped error", func() {
					err := deconfigurer.DeconfigureHost(log, "thehost", "thebridge")
					Ω(err).Should(MatchError(&network.DeleteLinkError{Cause: errors.New("ono"), Role: "bridge", Name: "thebridge"}))
				})
			})
		})

		Context("when the host device does not exist", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					if name == "thebridge" {
						return &net.Interface{Name: "thebridge"}, true, nil
					}

					return nil, false, nil
				}
			})

			It("still destroys the bridge", func() {
				Ω(deconfigurer.DeconfigureHost(log, "thehost", "thebridge")).Should(Succeed())
				Ω(fakeBridgeDeleter.DeleteCalledWith).Should(ContainElement(&net.Interface{Name: "thebridge"}))
			})
		})

		Context("when looking up the host returns an error", func() {
			BeforeEach(func() {
				fakeLink.InterfaceByNameFunc = func(name string) (*net.Interface, bool, error) {
					if name == "thebridge" {
						return &net.Interface{Name: "thebridge"}, true, nil
					}

					return nil, false, errors.New("o no")
				}
			})

			It("does not destroy the bridge", func() {
				Ω(deconfigurer.DeconfigureHost(log, "thehost", "thebridge")).ShouldNot(Succeed())
				Ω(fakeBridgeDeleter.DeleteCalledWith).Should(BeEmpty())
			})
		})
	})
})
