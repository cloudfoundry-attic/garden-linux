package bridgemgr_test

import (
	"errors"
	"net"

	"github.com/cloudfoundry-incubator/garden-linux/network/bridgemgr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BridgeNamePool", func() {
	var subnet1 *net.IPNet
	var subnet2 *net.IPNet
	var fakeDestroyer *destroyer
	var fakeLister *lister

	var mgr bridgemgr.BridgeManager

	BeforeEach(func() {
		_, subnet1, _ = net.ParseCIDR("1.2.3.4/30")
		_, subnet2, _ = net.ParseCIDR("1.2.3.4/29")

		fakeLister = &lister{}
		fakeDestroyer = &destroyer{}
		mgr = bridgemgr.New("pr", fakeDestroyer, fakeLister)
	})

	Describe("reserving", func() {
		Context("when no bridge names have been assigned", func() {
			It("assigns a prefixed name", func() {
				name, err := mgr.Reserve(subnet1, "container1")

				Ω(err).ShouldNot(HaveOccurred())
				Ω(name).Should(MatchRegexp("^pr"))
			})
		})

		Context("when a subnet has already been assigned a bridge name", func() {
			It("reuses the same name", func() {
				name1, err := mgr.Reserve(subnet1, "container1")
				Ω(err).ShouldNot(HaveOccurred())

				name2, err := mgr.Reserve(subnet1, "container2")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(name2).Should(Equal(name1))
			})
		})

		Context("when a bridge is acquired for a different subnet", func() {
			It("assigns a new bridge name", func() {
				name1, err := mgr.Reserve(subnet1, "container1")
				Ω(err).ShouldNot(HaveOccurred())

				name2, err := mgr.Reserve(subnet2, "container2")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(name2).ShouldNot(Equal(name1))
			})
		})
	})

	Describe("releasing", func() {
		Context("when a container releases its bridge", func() {
			var name string

			Context("and there are still containers in the subnet", func() {
				BeforeEach(func() {
					_, err := mgr.Reserve(subnet1, "container1")
					Ω(err).ShouldNot(HaveOccurred())
					name, err = mgr.Reserve(subnet1, "container2")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(mgr.Release(name, "container1")).Should(Succeed())
				})

				It("reuses the existing bridge name on the next Reserve", func() {
					newName, err := mgr.Reserve(subnet1, "container3")
					Ω(err).ShouldNot(HaveOccurred())
					Ω(newName).Should(Equal(name))
				})

				It("does not destroy the bridge", func() {
					Ω(fakeDestroyer.Destroyed).ShouldNot(ContainElement(name))
				})
			})

			Context("and it is the final container using the bridge", func() {
				var name string
				BeforeEach(func() {
					var err error
					name, err = mgr.Reserve(subnet1, "container1")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(mgr.Release(name, "container1")).Should(Succeed())
				})

				It("assigns a new bridge name on the next Reserve", func() {
					newName, err := mgr.Reserve(subnet1, "container2")
					Ω(err).ShouldNot(HaveOccurred())
					Ω(newName).ShouldNot(Equal(name))
				})

				It("destroys the bridge with the passed destroyer", func() {
					Ω(mgr.Release("some-bridge", "container1")).Should(Succeed())
					Ω(fakeDestroyer.Destroyed).Should(ContainElement("some-bridge"))
				})

				Context("when the destroyer returns an error", func() {
					It("returns an error", func() {
						fakeDestroyer.DestroyReturns = errors.New("bboom ")
						Ω(mgr.Release("some-bridge", "container1")).ShouldNot(Succeed())
					})
				})
			})

			Context("and it has not previously been acquired (e.g. when releasing an unknown bridge during recovery)", func() {
				It("destroys the bridge with the passed destroyer", func() {
					Ω(mgr.Release("some-bridge", "container1")).Should(Succeed())
					Ω(fakeDestroyer.Destroyed).Should(ContainElement("some-bridge"))
				})
			})
		})
	})

	Describe("rereserving", func() {
		It("returns an error if the bridge name is empty", func() {
			Ω(mgr.Rereserve("", subnet1, "some-id")).Should(MatchError("bridgemgr: re-reserving bridge: bridge name must not be empty"))
		})

		Context("when a bridge name is rereserved", func() {
			It("returns an error if the reacquired subnet is already assigned to another bridge name", func() {
				name, err := mgr.Reserve(subnet1, "")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(mgr.Rereserve(name, subnet2, "")).ShouldNot(Succeed())
			})

			Context("when the bridge could be reacquired", func() {
				BeforeEach(func() {
					Ω(mgr.Rereserve("my-bridge", subnet1, "my-container")).Should(Succeed())
				})

				Context("when a bridge name is acquired for the same subnet", func() {
					It("reuses the bridge name", func() {
						name, err := mgr.Reserve(subnet1, "another-container")
						Ω(err).ShouldNot(HaveOccurred())

						Ω(name).Should(Equal("my-bridge"))
					})

					Context("when it is released", func() {
						It("does not destroy the bridge, since the reacquired container is still using it", func() {
							Ω(mgr.Release("my-bridge", "another-container")).Should(Succeed())
							Ω(fakeDestroyer.Destroyed).ShouldNot(ContainElement("my-bridge"))
						})
					})
				})

				Context("when it is released", func() {
					It("destroys the bridge with the passed destroyer", func() {
						Ω(mgr.Release("my-bridge", "my-container")).Should(Succeed())
						Ω(fakeDestroyer.Destroyed).Should(ContainElement("my-bridge"))
					})
				})
			})
		})
	})

	Describe("pruning", func() {
		Context("when listing bridges fails", func() {
			BeforeEach(func() {
				fakeLister.ListReturns = errors.New("o no")
			})

			It("returns a wrapped error", func() {
				Ω(mgr.Prune()).Should(MatchError("bridgemgr: pruning bridges: o no"))
			})
		})

		Context("when there are no bridges", func() {
			It("does not destroy any bridges", func() {
				Ω(mgr.Prune()).Should(Succeed())
				Ω(fakeDestroyer.Destroyed).Should(HaveLen(0))
			})
		})

		Context("when there are multiple bridges", func() {
			BeforeEach(func() {
				fakeLister.Bridges = []string{"doesnotmatch", "pr-123", "pr-234"}
				mgr.Rereserve("pr-234", subnet1, "somecontainerid")
				Ω(mgr.Prune()).Should(Succeed())
			})

			It("destroys bridges with the prefix", func() {
				Ω(fakeDestroyer.Destroyed).Should(ContainElement("pr-123"))
			})

			It("does not destroy bridges without the prefix", func() {
				Ω(fakeDestroyer.Destroyed).ShouldNot(ContainElement("doesnotmatch"))
			})

			It("does not destroy bridges which are reserved", func() {
				Ω(fakeDestroyer.Destroyed).ShouldNot(ContainElement("pr-234"))
			})
		})
	})
})

type lister struct {
	Bridges     []string
	ListReturns error
}

func (l *lister) List() ([]string, error) {
	return l.Bridges, l.ListReturns
}

type destroyer struct {
	Destroyed      []string
	DestroyReturns error
}

func (d *destroyer) Destroy(name string) error {
	d.Destroyed = append(d.Destroyed, name)
	return d.DestroyReturns
}
