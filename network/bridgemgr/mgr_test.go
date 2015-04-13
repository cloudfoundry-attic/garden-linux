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
	var fakeBuilder *builder
	var fakeLister *lister

	var mgr bridgemgr.BridgeManager

	BeforeEach(func() {
		_, subnet1, _ = net.ParseCIDR("1.2.3.4/30")
		_, subnet2, _ = net.ParseCIDR("1.2.3.4/29")

		fakeBuilder = &builder{}
		fakeLister = &lister{}
		mgr = bridgemgr.New("pr", fakeBuilder, fakeLister)
	})

	Describe("reserving", func() {
		Context("when no bridge names have been assigned", func() {
			It("assigns a prefixed name", func() {
				name, err := mgr.Reserve(subnet1, "container1")

				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(MatchRegexp("^pr"))
			})

			It("creates the bridge", func() {
				name, err := mgr.Reserve(subnet1, "container-name")
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeBuilder.CreatedBridges).To(ContainElement(createParams{
					name:   name,
					subnet: subnet1,
					ip:     net.ParseIP("1.2.3.6"),
				}))
			})

			Context("when creating the bridge fails", func() {
				BeforeEach(func() {
					fakeBuilder.CreateReturns = errors.New("Bananas")
				})

				It("returns the error", func() {
					_, err := mgr.Reserve(subnet1, "container1")
					Expect(err).To(HaveOccurred())
				})

				Context("when the same subnet is acquired later", func() {
					It("retries the creation", func() {
						mgr.Reserve(subnet1, "container1")
						mgr.Reserve(subnet1, "container1")

						Expect(fakeBuilder.CreatedBridges).To(HaveLen(2))
					})
				})
			})
		})

		Context("when a subnet has already been assigned a bridge name", func() {
			It("reuses the same name", func() {
				name1, err := mgr.Reserve(subnet1, "container1")
				Expect(err).ToNot(HaveOccurred())

				name2, err := mgr.Reserve(subnet1, "container2")
				Expect(err).ToNot(HaveOccurred())

				Expect(name2).To(Equal(name1))
				Expect(fakeBuilder.CreatedBridges).To(HaveLen(1))
			})
		})

		Context("when a bridge is acquired for a different subnet", func() {
			It("assigns a new bridge name", func() {
				name1, err := mgr.Reserve(subnet1, "container1")
				Expect(err).ToNot(HaveOccurred())

				name2, err := mgr.Reserve(subnet2, "container2")
				Expect(err).ToNot(HaveOccurred())

				Expect(name2).ToNot(Equal(name1))
				Expect(fakeBuilder.CreatedBridges).To(HaveLen(2))
			})
		})
	})

	Describe("releasing", func() {
		Context("when a container releases its bridge", func() {
			var name string

			Context("and there are still containers in the subnet", func() {
				BeforeEach(func() {
					_, err := mgr.Reserve(subnet1, "container1")
					Expect(err).ToNot(HaveOccurred())
					name, err = mgr.Reserve(subnet1, "container2")
					Expect(err).ToNot(HaveOccurred())

					Expect(mgr.Release(name, "container1")).To(Succeed())
				})

				It("reuses the existing bridge name on the next Reserve", func() {
					newName, err := mgr.Reserve(subnet1, "container3")
					Expect(err).ToNot(HaveOccurred())
					Expect(newName).To(Equal(name))
				})

				It("does not destroy the bridge", func() {
					Expect(fakeBuilder.Destroyed).ToNot(ContainElement(name))
				})
			})

			Context("and it is the final container using the bridge", func() {
				var name string
				BeforeEach(func() {
					var err error
					name, err = mgr.Reserve(subnet1, "container1")
					Expect(err).ToNot(HaveOccurred())

					Expect(mgr.Release(name, "container1")).To(Succeed())
				})

				It("assigns a new bridge name on the next Reserve", func() {
					newName, err := mgr.Reserve(subnet1, "container2")
					Expect(err).ToNot(HaveOccurred())
					Expect(newName).ToNot(Equal(name))
				})

				It("destroys the bridge with the passed destroyer", func() {
					Expect(mgr.Release("some-bridge", "container1")).To(Succeed())
					Expect(fakeBuilder.Destroyed).To(ContainElement("some-bridge"))
				})

				Context("when the destroyer returns an error", func() {
					It("returns an error", func() {
						fakeBuilder.DestroyReturns = errors.New("bboom ")
						Expect(mgr.Release("some-bridge", "container1")).ToNot(Succeed())
					})
				})
			})

			Context("and it has not previously been acquired (e.g. when releasing an unknown bridge during recovery)", func() {
				It("destroys the bridge with the passed destroyer", func() {
					Expect(mgr.Release("some-bridge", "container1")).To(Succeed())
					Expect(fakeBuilder.Destroyed).To(ContainElement("some-bridge"))
				})
			})
		})
	})

	Describe("rereserving", func() {
		It("returns an error if the bridge name is empty", func() {
			Expect(mgr.Rereserve("", subnet1, "some-id")).To(MatchError("bridgemgr: re-reserving bridge: bridge name must not be empty"))
		})

		Context("when a bridge name is rereserved", func() {
			It("returns an error if the reacquired subnet is already assigned to another bridge name", func() {
				name, err := mgr.Reserve(subnet1, "")
				Expect(err).ToNot(HaveOccurred())

				Expect(mgr.Rereserve(name, subnet2, "")).ToNot(Succeed())
			})

			Context("when the bridge could be reacquired", func() {
				BeforeEach(func() {
					Expect(mgr.Rereserve("my-bridge", subnet1, "my-container")).To(Succeed())
				})

				Context("when a bridge name is acquired for the same subnet", func() {
					It("reuses the bridge name", func() {
						name, err := mgr.Reserve(subnet1, "another-container")
						Expect(err).ToNot(HaveOccurred())

						Expect(name).To(Equal("my-bridge"))
					})

					Context("when it is released", func() {
						It("does not destroy the bridge, since the reacquired container is still using it", func() {
							Expect(mgr.Release("my-bridge", "another-container")).To(Succeed())
							Expect(fakeBuilder.Destroyed).ToNot(ContainElement("my-bridge"))
						})
					})
				})

				Context("when it is released", func() {
					It("destroys the bridge with the passed destroyer", func() {
						Expect(mgr.Release("my-bridge", "my-container")).To(Succeed())
						Expect(fakeBuilder.Destroyed).To(ContainElement("my-bridge"))
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
				Expect(mgr.Prune()).To(MatchError("bridgemgr: pruning bridges: o no"))
			})
		})

		Context("when there are no bridges", func() {
			It("does not destroy any bridges", func() {
				Expect(mgr.Prune()).To(Succeed())
				Expect(fakeBuilder.Destroyed).To(HaveLen(0))
			})
		})

		Context("when there are multiple bridges", func() {
			BeforeEach(func() {
				fakeLister.Bridges = []string{"doesnotmatch", "pr-123", "pr-234"}
				mgr.Rereserve("pr-234", subnet1, "somecontainerid")
				Expect(mgr.Prune()).To(Succeed())
			})

			It("destroys bridges with the prefix", func() {
				Expect(fakeBuilder.Destroyed).To(ContainElement("pr-123"))
			})

			It("does not destroy bridges without the prefix", func() {
				Expect(fakeBuilder.Destroyed).ToNot(ContainElement("doesnotmatch"))
			})

			It("does not destroy bridges which are reserved", func() {
				Expect(fakeBuilder.Destroyed).ToNot(ContainElement("pr-234"))
			})
		})
	})
})

type builder struct {
	CreatedBridges []createParams
	CreateReturns  error
	Destroyed      []string
	DestroyReturns error
}

type createParams struct {
	name   string
	subnet *net.IPNet
	ip     net.IP
}

func (c *builder) Create(name string, ip net.IP, subnet *net.IPNet) (*net.Interface, error) {
	c.CreatedBridges = append(c.CreatedBridges, createParams{
		name:   name,
		subnet: subnet,
		ip:     ip,
	})

	return nil, c.CreateReturns
}

func (d *builder) Destroy(name string) error {
	d.Destroyed = append(d.Destroyed, name)
	return d.DestroyReturns
}

type lister struct {
	Bridges     []string
	ListReturns error
}

func (l *lister) List() ([]string, error) {
	return l.Bridges, l.ListReturns
}
