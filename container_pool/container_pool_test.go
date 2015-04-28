package container_pool_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden-linux/container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/container_pool/fake_container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/container_pool/fake_subnet_pool"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/bridgemgr/fake_bridge_manager"
	"github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/fake_rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/old/uid_pool/fake_uid_pool"
	"github.com/cloudfoundry-incubator/garden-linux/process"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Container pool", func() {
	var depotPath string
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var fakeUIDPool *fake_uid_pool.FakeUIDPool
	var fakeSubnetPool *fake_subnet_pool.FakeSubnetPool
	var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
	var fakePortPool *fake_port_pool.FakePortPool
	var defaultFakeRootFSProvider *fake_rootfs_provider.FakeRootFSProvider
	var fakeRootFSProvider *fake_rootfs_provider.FakeRootFSProvider
	var fakeBridges *fake_bridge_manager.FakeBridgeManager
	var fakeFilterProvider *fake_container_pool.FakeFilterProvider
	var fakeFilter *fakes.FakeFilter
	var pool *container_pool.LinuxContainerPool
	var config sysconfig.Config

	var containerNetwork *linux_backend.Network

	BeforeEach(func() {
		fakeUIDPool = fake_uid_pool.New(10000)
		fakeSubnetPool = new(fake_subnet_pool.FakeSubnetPool)

		var err error
		containerNetwork = &linux_backend.Network{}
		containerNetwork.IP, containerNetwork.Subnet, err = net.ParseCIDR("10.2.0.1/30")
		Expect(err).ToNot(HaveOccurred())
		fakeSubnetPool.AcquireReturns(containerNetwork, nil)

		fakeBridges = new(fake_bridge_manager.FakeBridgeManager)

		fakeBridges.ReserveStub = func(n *net.IPNet, c string) (string, error) {
			return fmt.Sprintf("bridge-for-%s-%s", n, c), nil
		}

		fakeFilter = new(fakes.FakeFilter)
		fakeFilterProvider = new(fake_container_pool.FakeFilterProvider)
		fakeFilterProvider.ProvideFilterStub = func(id string) network.Filter {
			return fakeFilter
		}

		fakeRunner = fake_command_runner.New()
		fakeQuotaManager = fake_quota_manager.New()
		fakePortPool = fake_port_pool.New(1000)
		defaultFakeRootFSProvider = new(fake_rootfs_provider.FakeRootFSProvider)
		fakeRootFSProvider = new(fake_rootfs_provider.FakeRootFSProvider)

		defaultFakeRootFSProvider.ProvideRootFSReturns("/provided/rootfs/path", nil, nil)

		depotPath, err = ioutil.TempDir("", "depot-path")
		Expect(err).ToNot(HaveOccurred())

		config = sysconfig.NewConfig("0", false)
		logger := lagertest.NewTestLogger("test")
		pool = container_pool.New(
			logger,
			"/root/path",
			depotPath,
			config,
			map[string]rootfs_provider.RootFSProvider{
				"":     defaultFakeRootFSProvider,
				"fake": fakeRootFSProvider,
			},
			fakeUIDPool,
			net.ParseIP("1.2.3.4"),
			345,
			fakeSubnetPool,
			fakeBridges,
			fakeFilterProvider,
			iptables.NewGlobalChain("global-default-chain", fakeRunner, logger),
			fakePortPool,
			[]string{"1.1.0.0/16", "", "2.2.0.0/16"}, // empty string to test that this is ignored
			[]string{"1.1.1.1/32", "", "2.2.2.2/32"},
			fakeRunner,
			fakeQuotaManager,
		)
	})

	AfterEach(func() {
		os.RemoveAll(depotPath)
	})

	Describe("MaxContainer", func() {
		Context("when constrained by network pool size", func() {
			BeforeEach(func() {
				fakeSubnetPool.CapacityReturns(5)
				fakeUIDPool.InitialPoolSize = 3000
			})

			It("returns the network pool size", func() {
				Expect(pool.MaxContainers()).To(Equal(5))
			})
		})
		Context("when constrained by uid pool size", func() {
			BeforeEach(func() {
				fakeSubnetPool.CapacityReturns(666)
				fakeUIDPool.InitialPoolSize = 42
			})

			It("returns the uid pool size", func() {
				Expect(pool.MaxContainers()).To(Equal(42))
			})
		})
	})

	Describe("Setup", func() {
		It("executes setup.sh with the correct environment", func() {
			fakeQuotaManager.MountPointResult = "/depot/mount/point"

			err := pool.Setup()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/root/path/setup.sh",
					Env: []string{
						"CONTAINER_DEPOT_PATH=" + depotPath,
						"CONTAINER_DEPOT_MOUNT_POINT_PATH=/depot/mount/point",
						"DISK_QUOTA_ENABLED=true",

						"PATH=" + os.Getenv("PATH"),
					},
				},
			))
		})

		Context("when setup.sh fails", func() {
			nastyError := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/root/path/setup.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := pool.Setup()
				Expect(err).To(Equal(nastyError))
			})
		})

		Describe("Setting up IPTables", func() {
			It("sets up global allow and deny rules, adding allow before deny", func() {
				err := pool.Setup()
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/setup.sh", // must run iptables rules after setup.sh
					},
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "global-default-chain", "--destination", "1.1.1.1/32", "--jump", "RETURN"},
					},
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "global-default-chain", "--destination", "2.2.2.2/32", "--jump", "RETURN"},
					},
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "global-default-chain", "--destination", "1.1.0.0/16", "--jump", "REJECT"},
					},
					fake_command_runner.CommandSpec{
						Path: "/sbin/iptables",
						Args: []string{"-w", "-A", "global-default-chain", "--destination", "2.2.0.0/16", "--jump", "REJECT"},
					},
				))
			})

			Context("when setting up a rule fails", func() {
				nastyError := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: "/sbin/iptables",
						}, func(*exec.Cmd) error {
							return nastyError
						},
					)
				})

				It("returns a wrapped error", func() {
					err := pool.Setup()
					Expect(err).To(MatchError("container_pool: setting up allow rules in iptables: oh no!"))
				})
			})
		})
	})

	Describe("creating", func() {
		itReleasesTheUserIDs := func() {
			It("returns the container's user ID and root ID to the pool", func() {
				Expect(fakeUIDPool.Released).To(Equal([]uint32{10000, 10001}))
			})
		}

		itReleasesTheIPBlock := func() {
			It("returns the container's IP block to the pool", func() {
				Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(1))
				Expect(fakeSubnetPool.ReleaseArgsForCall(0)).To(Equal(containerNetwork))
			})
		}

		itDeletesTheContainerDirectory := func() {
			It("deletes the container's directory", func() {
				executedCommands := fakeRunner.ExecutedCommands()

				createCommand := executedCommands[0]
				Expect(createCommand.Path).To(Equal("/root/path/create.sh"))
				containerPath := createCommand.Args[1]

				lastCommand := executedCommands[len(executedCommands)-1]
				Expect(lastCommand.Path).To(Equal("/root/path/destroy.sh"))
				Expect(lastCommand.Args[1]).To(Equal(containerPath))
			})
		}

		itCleansUpTheRootfs := func() {
			It("cleans up the rootfs for the container", func() {
				Expect(defaultFakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(1))
				_, providedID, _ := defaultFakeRootFSProvider.ProvideRootFSArgsForCall(0)
				_, cleanedUpID := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Expect(cleanedUpID).To(Equal(providedID))
			})
		}

		itReleasesAndDestroysTheBridge := func() {
			It("releases the bridge", func() {
				Expect(fakeBridges.ReleaseCallCount()).To(Equal(1))
				_, containerId := fakeBridges.ReserveArgsForCall(0)

				Expect(fakeBridges.ReleaseCallCount()).To(Equal(1))
				bridgeName, containerId := fakeBridges.ReleaseArgsForCall(0)
				Expect(bridgeName).To(Equal("bridge-for-10.2.0.0/30-" + containerId))
				Expect(containerId).To(Equal(containerId))
			})
		}

		It("returns containers with unique IDs", func() {
			container1, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			container2, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			Expect(container1.ID()).ToNot(Equal(container2.ID()))
		})

		It("creates containers with the correct grace time", func() {
			container, err := pool.Create(garden.ContainerSpec{
				GraceTime: 1 * time.Second,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.GraceTime()).To(Equal(1 * time.Second))
		})

		It("creates containers with the correct properties", func() {
			properties := garden.Properties(map[string]string{
				"foo": "bar",
			})

			container, err := pool.Create(garden.ContainerSpec{
				Properties: properties,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.Properties()).To(Equal(properties))
		})

		It("sets up iptable filters for the container", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeFilterProvider.ProvideFilterCallCount()).To(BeNumerically(">", 0))
			Expect(fakeFilterProvider.ProvideFilterArgsForCall(0)).To(Equal(container.Handle()))
			Expect(fakeFilter.SetupCallCount()).To(Equal(1))
			Expect(fakeFilter.SetupArgsForCall(0)).To(Equal(container.Handle()))
		})

		Context("when setting up iptables fails", func() {
			var err error
			BeforeEach(func() {
				fakeFilter.SetupReturns(errors.New("iptables says no"))
				_, err = pool.Create(garden.ContainerSpec{})
				Expect(err).To(HaveOccurred())
			})

			It("returns a wrapped error", func() {
				Expect(err).To(MatchError("container_pool: set up filter: iptables says no"))
			})

			itReleasesTheUserIDs()
			itReleasesTheIPBlock()
			itCleansUpTheRootfs()
			itDeletesTheContainerDirectory()
		})

		Context("when the privileged flag is specified and true", func() {
			It("executes create.sh with a root_uid of 0", func() {
				container, err := pool.Create(garden.ContainerSpec{Privileged: true})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
						Args: []string{path.Join(depotPath, container.ID())},
						Env: []string{
							"PATH=" + os.Getenv("PATH"),
							"bridge_iface=bridge-for-10.2.0.0/30-" + container.ID(),
							"container_iface_mtu=345",
							"external_ip=1.2.3.4",
							"id=" + container.ID(),
							"network_cidr=10.2.0.0/30",
							"network_cidr_suffix=30",
							"network_container_ip=10.2.0.1",
							"network_host_ip=10.2.0.2",
							"root_uid=0",
							"rootfs_path=/provided/rootfs/path",
							"user_uid=10000",
						},
					},
				))
			})
		})

		Context("when no Network parameter is specified", func() {
			It("executes create.sh with the correct args and environment", func() {
				container, err := pool.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
						Args: []string{path.Join(depotPath, container.ID())},
						Env: []string{
							"PATH=" + os.Getenv("PATH"),
							"bridge_iface=bridge-for-10.2.0.0/30-" + container.ID(),
							"container_iface_mtu=345",
							"external_ip=1.2.3.4",
							"id=" + container.ID(),
							"network_cidr=10.2.0.0/30",
							"network_cidr_suffix=30",
							"network_container_ip=10.2.0.1",
							"network_host_ip=10.2.0.2",
							"root_uid=10001",
							"rootfs_path=/provided/rootfs/path",
							"user_uid=10000",
						},
					},
				))
			})
		})

		Context("when the Network parameter is specified", func() {
			It("executes create.sh with the correct args and environment", func() {
				differentNetwork := &linux_backend.Network{}
				differentNetwork.IP, differentNetwork.Subnet, _ = net.ParseCIDR("10.3.0.2/29")
				fakeSubnetPool.AcquireReturns(differentNetwork, nil)

				container, err := pool.Create(garden.ContainerSpec{
					Network: "1.3.0.0/30",
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
						Args: []string{path.Join(depotPath, container.ID())},
						Env: []string{
							"PATH=" + os.Getenv("PATH"),
							"bridge_iface=bridge-for-10.3.0.0/29-" + container.ID(),
							"container_iface_mtu=345",
							"external_ip=1.2.3.4",
							"id=" + container.ID(),
							"network_cidr=10.3.0.0/29",
							"network_cidr_suffix=29",
							"network_container_ip=10.3.0.2",
							"network_host_ip=10.3.0.6",
							"root_uid=10001",
							"rootfs_path=/provided/rootfs/path",
							"user_uid=10000",
						},
					},
				))
			})

			It("creates the container directory", func() {
				container, err := pool.Create(garden.ContainerSpec{})
				Expect(err).To(Succeed())

				containerDir := path.Join(depotPath, container.ID())
				_, err = os.Stat(containerDir)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when creating the container directory fails", func() {
				JustBeforeEach(func() {
					Expect(os.Remove(depotPath)).To(Succeed())
					ioutil.WriteFile(depotPath, []byte(""), 0755)
				})

				It("returns an error", func() {
					_, err := pool.Create(garden.ContainerSpec{})
					Expect(err).To(MatchError(HavePrefix("containerpool: creating container directory")))
				})
			})

			Describe("allocating the requested network", func() {
				itShouldAcquire := func(subnet subnets.SubnetSelector, ip subnets.IPSelector) {
					Expect(fakeSubnetPool.AcquireCallCount()).To(Equal(1))
					s, i := fakeSubnetPool.AcquireArgsForCall(0)

					Expect(s).To(Equal(subnet))
					Expect(i).To(Equal(ip))
				}

				Context("when the network string is empty", func() {
					It("allocates a dynamic subnet and ip", func() {
						_, err := pool.Create(garden.ContainerSpec{Network: ""})
						Expect(err).ToNot(HaveOccurred())

						itShouldAcquire(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
					})
				})

				Context("when the network parameter is not empty", func() {
					Context("when it contains a prefix length", func() {
						It("statically allocates the requested subnet ", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0/30"})
							Expect(err).ToNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/30")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when it does not contain a prefix length", func() {
						It("statically allocates the requested Network from Subnets as a /30", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0"})
							Expect(err).ToNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/30")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when the network parameter has non-zero host bits", func() {
						It("statically allocates an IP address based on the network parameter", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.1/20"})
							Expect(err).ToNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/20")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.StaticIPSelector{net.ParseIP("1.2.3.1")})
						})
					})

					Context("when the network parameter has zero host bits", func() {
						It("dynamically allocates an IP address", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0/24"})
							Expect(err).ToNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/24")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when an invalid network string is passed", func() {
						It("returns an error", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "not a network"})
							Expect(err).To(MatchError("create container: invalid network spec: invalid CIDR address: not a network/30"))
						})

						It("does not acquire any resources", func() {
							Expect(fakeUIDPool.Acquired).To(HaveLen(0))
							Expect(fakePortPool.Acquired).To(HaveLen(0))
							Expect(fakeSubnetPool.AcquireCallCount()).To(Equal(0))
						})
					})
				})
			})

			Context("when allocation of the specified Network fails", func() {
				var err error
				allocateError := errors.New("allocateError")

				BeforeEach(func() {
					fakeSubnetPool.AcquireReturns(nil, allocateError)

					_, err = pool.Create(garden.ContainerSpec{
						Network: "1.2.0.0/30",
					})
				})

				It("returns the error", func() {
					Expect(err).To(Equal(allocateError))
				})

				itReleasesTheUserIDs()

				It("does not execute create.sh", func() {
					Expect(fakeRunner).ToNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/create.sh",
						},
					))
				})

				It("doesn't attempt to release the network if it has not been assigned", func() {
					Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(0))
				})
			})
		})

		It("saves the bridge name to the depot", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "bridge-name"))
			Expect(err).ToNot(HaveOccurred())

			Expect(string(body)).To(Equal("bridge-for-10.2.0.0/30-" + container.ID()))
		})

		It("saves the determined rootfs provider to the depot", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
			Expect(err).ToNot(HaveOccurred())

			Expect(string(body)).To(Equal(""))
		})

		Context("when a rootfs is specified", func() {
			It("is used to provide a rootfs", func() {
				container, err := pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				_, id, uri := fakeRootFSProvider.ProvideRootFSArgsForCall(0)
				Expect(id).To(Equal(container.ID()))
				Expect(uri).To(Equal(&url.URL{
					Scheme: "fake",
					Host:   "",
					Path:   "/path/to/custom-rootfs",
				}))
			})

			It("passes the provided rootfs as $rootfs_path to create.sh", func() {
				fakeRootFSProvider.ProvideRootFSReturns("/var/some/mount/point", nil, nil)

				container, err := pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
						Args: []string{path.Join(depotPath, container.ID())},
						Env: []string{
							"PATH=" + os.Getenv("PATH"),
							"bridge_iface=bridge-for-10.2.0.0/30-" + container.ID(),
							"container_iface_mtu=345",
							"external_ip=1.2.3.4",
							"id=" + container.ID(),
							"network_cidr=10.2.0.0/30",
							"network_cidr_suffix=30",
							"network_container_ip=10.2.0.1",
							"network_host_ip=10.2.0.2",
							"root_uid=10001",
							"rootfs_path=/var/some/mount/point",
							"user_uid=10000",
						},
					},
				))
			})

			It("saves the determined rootfs provider to the depot", func() {
				container, err := pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
				Expect(err).ToNot(HaveOccurred())

				Expect(string(body)).To(Equal("fake"))
			})

			It("returns an error if the supplied environment is invalid", func() {
				_, err := pool.Create(garden.ContainerSpec{
					Env: []string{
						"hello",
					},
				})
				Expect(err).To(MatchError(HavePrefix("process: malformed environment")))
			})

			It("merges the env vars associated with the rootfs with those in the spec", func() {
				fakeRootFSProvider.ProvideRootFSReturns("/provided/rootfs/path", process.Env{
					"var2": "rootfs-value-2",
					"var3": "rootfs-value-3",
				}, nil)

				container, err := pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
					Env: []string{
						"var1=spec-value1",
						"var2=spec-value2",
					},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(container.(*linux_container.LinuxContainer).CurrentEnvVars()).To(Equal(process.Env{
					"var1": "spec-value1",
					"var2": "spec-value2",
					"var3": "rootfs-value-3",
				}))
			})

			Context("when the rootfs URL is not valid", func() {
				var err error

				BeforeEach(func() {
					_, err = pool.Create(garden.ContainerSpec{
						RootFSPath: "::::::",
					})
				})

				It("returns an error", func() {
					Expect(err).To(BeAssignableToTypeOf(&url.Error{}))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Expect(fakeBridges.ReserveCallCount()).To(Equal(0))
				})
			})

			Context("when its scheme is unknown", func() {
				var err error

				BeforeEach(func() {
					_, err = pool.Create(garden.ContainerSpec{
						RootFSPath: "unknown:///path/to/custom-rootfs",
					})
				})

				It("returns ErrUnknownRootFSProvider", func() {
					Expect(err).To(Equal(container_pool.ErrUnknownRootFSProvider))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Expect(fakeBridges.ReserveCallCount()).To(Equal(0))
				})
			})

			Context("when providing the mount point fails", func() {
				var err error
				providerErr := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.ProvideRootFSReturns("", nil, providerErr)

					_, err = pool.Create(garden.ContainerSpec{
						RootFSPath: "fake:///path/to/custom-rootfs",
					})
				})

				It("returns the error", func() {
					Expect(err).To(Equal(providerErr))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Expect(fakeBridges.ReserveCallCount()).To(Equal(0))
				})

				It("does not execute create.sh", func() {
					Expect(fakeRunner).ToNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/create.sh",
						},
					))
				})
			})
		})

		Context("when acquiring the bridge fails", func() {
			var err error
			BeforeEach(func() {
				fakeRootFSProvider.ProvideRootFSReturns("the-rootfs", nil, nil)
				fakeBridges.ReserveReturns("", errors.New("o no"))
				_, err = pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
			})

			It("does not execute create.sh", func() {
				Expect(fakeRunner).ToNot(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
					},
				))
			})

			It("cleans up the rootfs", func() {
				Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(1))
				_, rootfsPath := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Expect(rootfsPath).To(Equal("the-rootfs"))
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when bind mounts are specified", func() {
			It("appends mount commands to hook-parent-before-clone.sh", func() {
				container, err := pool.Create(garden.ContainerSpec{
					BindMounts: []garden.BindMount{
						{
							SrcPath: "/src/path-ro",
							DstPath: "/dst/path-ro",
							Mode:    garden.BindMountModeRO,
						},
						{
							SrcPath: "/src/path-rw",
							DstPath: "/dst/path-rw",
							Mode:    garden.BindMountModeRW,
						},
						{
							SrcPath: "/src/path-rw",
							DstPath: "/dst/path-rw",
							Mode:    garden.BindMountModeRW,
							Origin:  garden.BindMountOriginContainer,
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())

				containerPath := path.Join(depotPath, container.ID())
				rootfsPath := "/provided/rootfs/path"

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + rootfsPath + "/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind /src/path-ro " + rootfsPath + "/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,ro /src/path-ro " + rootfsPath + "/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind /src/path-rw " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,rw /src/path-rw " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind " + rootfsPath + "/src/path-rw " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,rw " + rootfsPath + "/src/path-rw " + rootfsPath + "/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-parent-before-clone.sh",
						},
					},
				))
			})

			Context("when appending to hook-parent-before-clone.sh", func() {
				var err error
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
						Path: "bash",
					}, func(*exec.Cmd) error {
						return disaster
					})

					_, err = pool.Create(garden.ContainerSpec{
						BindMounts: []garden.BindMount{
							{
								SrcPath: "/src/path-ro",
								DstPath: "/dst/path-ro",
								Mode:    garden.BindMountModeRO,
							},
							{
								SrcPath: "/src/path-rw",
								DstPath: "/dst/path-rw",
								Mode:    garden.BindMountModeRW,
							},
						},
					})
				})

				It("returns the error", func() {
					Expect(err).To(Equal(disaster))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()
				itCleansUpTheRootfs()
				itDeletesTheContainerDirectory()
			})
		})

		Context("when acquiring a UID fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeUIDPool.AcquireError = nastyError
			})

			It("returns the error", func() {
				_, err := pool.Create(garden.ContainerSpec{})
				Expect(err).To(Equal(nastyError))
			})
		})

		Context("when executing create.sh fails", func() {
			nastyError := errors.New("oh no!")
			var err error

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
					}, func(cmd *exec.Cmd) error {
						return nastyError
					},
				)

				_, err = pool.Create(garden.ContainerSpec{})
			})

			It("returns the error and releases the uid and network", func() {
				Expect(err).To(Equal(nastyError))

				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))

				Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(1))
				Expect(fakeSubnetPool.ReleaseArgsForCall(0)).To(Equal(containerNetwork))
			})

			itReleasesTheUserIDs()
			itReleasesTheIPBlock()
			itDeletesTheContainerDirectory()
			itCleansUpTheRootfs()
			itReleasesAndDestroysTheBridge()
		})

		Context("when saving the rootfs provider fails", func() {
			var err error

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
					}, func(cmd *exec.Cmd) error {
						containerPath := cmd.Args[1]
						rootfsProviderPath := filepath.Join(containerPath, "rootfs-provider")

						// creating a directory with this name will cause the write to the
						// file to fail.
						err := os.MkdirAll(rootfsProviderPath, 0755)
						Expect(err).ToNot(HaveOccurred())

						return nil
					},
				)

				_, err = pool.Create(garden.ContainerSpec{})
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})

			itReleasesTheUserIDs()
			itReleasesTheIPBlock()
			itCleansUpTheRootfs()
			itDeletesTheContainerDirectory()
		})
	})

	Describe("restoring", func() {
		var snapshot io.Reader
		var buf *bytes.Buffer

		var containerNetwork *linux_backend.Network
		var rootUID uint32
		var bridgeName string

		BeforeEach(func() {
			rootUID = 10001

			buf = new(bytes.Buffer)
			snapshot = buf
			_, subnet, _ := net.ParseCIDR("2.3.4.5/29")
			containerNetwork = &linux_backend.Network{
				Subnet: subnet,
				IP:     net.ParseIP("1.2.3.4"),
			}

			bridgeName = "some-bridge"
		})

		JustBeforeEach(func() {
			err := json.NewEncoder(buf).Encode(
				linux_container.ContainerSnapshot{
					ID:     "some-restored-id",
					Handle: "some-restored-handle",

					GraceTime: 1 * time.Second,

					State: "some-restored-state",
					Events: []string{
						"some-restored-event",
						"some-other-restored-event",
					},

					Resources: linux_container.ResourcesSnapshot{
						UserUID: 10000,
						RootUID: rootUID,
						Network: containerNetwork,
						Bridge:  bridgeName,
						Ports:   []uint32{61001, 61002, 61003},
					},

					Properties: map[string]string{
						"foo": "bar",
					},
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("constructs a container from the snapshot", func() {
			container, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(container.ID()).To(Equal("some-restored-id"))
			Expect(container.Handle()).To(Equal("some-restored-handle"))
			Expect(container.GraceTime()).To(Equal(1 * time.Second))
			Expect(container.Properties()).To(Equal(garden.Properties(map[string]string{
				"foo": "bar",
			})))

			linuxContainer := container.(*linux_container.LinuxContainer)

			Expect(linuxContainer.State()).To(Equal(linux_container.State("some-restored-state")))
			Expect(linuxContainer.Events()).To(Equal([]string{
				"some-restored-event",
				"some-other-restored-event",
			}))

			Expect(linuxContainer.Resources().Network).To(Equal(containerNetwork))
			Expect(linuxContainer.Resources().Bridge).To(Equal("some-bridge"))
		})

		It("removes its UID from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeUIDPool.Removed).To(ContainElement(uint32(10000)))
		})

		Context("when the Root UID is 0", func() {
			BeforeEach(func() {
				rootUID = 0
			})

			It("does not remove it from the pool", func() {
				_, err := pool.Restore(snapshot)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeUIDPool.Removed).ToNot(ContainElement(rootUID))
			})
		})

		It("removes its network from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeSubnetPool.RemoveCallCount()).To(Equal(1))
			Expect(fakeSubnetPool.RemoveArgsForCall(0)).To(Equal(containerNetwork))
		})

		It("removes its ports from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakePortPool.Removed).To(ContainElement(uint32(61001)))
			Expect(fakePortPool.Removed).To(ContainElement(uint32(61002)))
			Expect(fakePortPool.Removed).To(ContainElement(uint32(61003)))
		})

		It("rereserves the bridge for the subnet from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeBridges.RereserveCallCount()).To(Equal(1))
			bridgeName, subnet, containerId := fakeBridges.RereserveArgsForCall(0)
			Expect(bridgeName).To(Equal("some-bridge"))
			Expect(subnet.String()).To(Equal("2.3.4.0/29"))
			Expect(containerId).To(Equal("some-restored-id"))
		})

		Context("when rereserving the bridge fails", func() {
			var err error

			JustBeforeEach(func() {
				fakeBridges.RereserveReturns(errors.New("boom"))
				_, err = pool.Restore(snapshot)
			})

			It("returns the error", func() {
				Expect(err).To(HaveOccurred())
			})

			It("returns the UID to the pool", func() {
				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))
			})

			It("returns the subnet to the pool", func() {
				Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(1))
				Expect(fakeSubnetPool.ReleaseArgsForCall(0)).To(Equal(containerNetwork))
			})
		})

		Context("when decoding the snapshot fails", func() {
			BeforeEach(func() {
				snapshot = new(bytes.Buffer)
			})

			It("fails", func() {
				_, err := pool.Restore(snapshot)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when removing the UID from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeUIDPool.RemoveError = disaster
			})

			It("returns the error", func() {
				_, err := pool.Restore(snapshot)
				Expect(err).To(Equal(disaster))
			})
		})

		Context("when removing the network from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeSubnetPool.RemoveReturns(disaster)
			})

			It("returns the error and releases the uid", func() {
				_, err := pool.Restore(snapshot)
				Expect(err).To(Equal(disaster))

				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))
			})
		})

		Context("when removing a port from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakePortPool.RemoveError = disaster
			})

			It("returns the error and releases the uid, network, and all ports", func() {
				_, err := pool.Restore(snapshot)
				Expect(err).To(Equal(disaster))

				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))

				Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(1))
				Expect(fakeSubnetPool.ReleaseArgsForCall(0)).To(Equal(containerNetwork))

				Expect(fakePortPool.Released).To(ContainElement(uint32(61001)))
				Expect(fakePortPool.Released).To(ContainElement(uint32(61002)))
				Expect(fakePortPool.Released).To(ContainElement(uint32(61003)))
			})

			Context("when the container is privileged", func() {
				BeforeEach(func() {
					rootUID = 0
				})

				It("does not release uid 0 back to the uid pool", func() {
					_, err := pool.Restore(snapshot)
					Expect(err).To(Equal(disaster))

					Expect(fakeUIDPool.Released).ToNot(ContainElement(rootUID))
				})
			})
		})
	})

	Describe("pruning", func() {
		Context("when containers are found in the depot", func() {
			BeforeEach(func() {
				err := os.MkdirAll(path.Join(depotPath, "container-1"), 0755)
				Expect(err).ToNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "container-2"), 0755)
				Expect(err).ToNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "container-3"), 0755)
				Expect(err).ToNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "tmp"), 0755)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-1", "bridge-name"), []byte("fake-bridge-1"), 0644)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-2", "bridge-name"), []byte("fake-bridge-2"), 0644)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-1", "rootfs-provider"), []byte("fake"), 0644)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-2", "rootfs-provider"), []byte("fake"), 0644)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-3", "rootfs-provider"), []byte(""), 0644)
				Expect(err).ToNot(HaveOccurred())
			})

			It("destroys each container", func() {
				err := pool.Prune(map[string]bool{})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/destroy.sh",
						Args: []string{path.Join(depotPath, "container-1")},
					},
					fake_command_runner.CommandSpec{
						Path: "/root/path/destroy.sh",
						Args: []string{path.Join(depotPath, "container-2")},
					},
					fake_command_runner.CommandSpec{
						Path: "/root/path/destroy.sh",
						Args: []string{path.Join(depotPath, "container-3")},
					},
				))
			})

			Context("after destroying it", func() {
				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: "/root/path/destroy.sh",
						}, func(cmd *exec.Cmd) error {
							return os.RemoveAll(cmd.Args[0])
						},
					)
				})

				It("cleans up each container's rootfs after destroying it", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(2))
					_, id1 := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
					_, id2 := fakeRootFSProvider.CleanupRootFSArgsForCall(1)
					Expect(id1).To(Equal("container-1"))
					Expect(id2).To(Equal("container-2"))

					Expect(defaultFakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(1))
					_, id3 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
					Expect(id3).To(Equal("container-3"))
				})

				It("releases the bridge", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeBridges.ReleaseCallCount()).To(Equal(2))

					bridge, containerId := fakeBridges.ReleaseArgsForCall(0)
					Expect(bridge).To(Equal("fake-bridge-1"))
					Expect(containerId).To(Equal("container-1"))

					bridge, containerId = fakeBridges.ReleaseArgsForCall(1)
					Expect(bridge).To(Equal("fake-bridge-2"))
					Expect(containerId).To(Equal("container-2"))
				})
			})

			Context("when a container does not declare a bridge name", func() {
				It("does nothing much", func() {
					err := pool.Prune(map[string]bool{"container-1": true, "container-2": true})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeBridges.ReleaseCallCount()).To(Equal(0))
				})
			})

			Context("when a container does not declare a rootfs provider", func() {
				BeforeEach(func() {
					err := os.Remove(path.Join(depotPath, "container-2", "rootfs-provider"))
					Expect(err).ToNot(HaveOccurred())
				})

				It("cleans it up using the default provider", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).ToNot(HaveOccurred())

					Expect(defaultFakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(2))
					_, id1 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
					_, id2 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(1)
					Expect(id1).To(Equal("container-2"))
					Expect(id2).To(Equal("container-3"))
				})

				Context("when a container exists with an unknown rootfs provider", func() {
					BeforeEach(func() {
						err := ioutil.WriteFile(path.Join(depotPath, "container-2", "rootfs-provider"), []byte("unknown"), 0644)
						Expect(err).ToNot(HaveOccurred())
					})

					It("ignores the error", func() {
						err := pool.Prune(map[string]bool{})
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			Context("when cleaning up the rootfs fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.CleanupRootFSReturns(disaster)
				})

				It("ignores the error", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when a container to keep is specified", func() {
				It("is not destroyed", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRunner).ToNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/destroy.sh",
							Args: []string{path.Join(depotPath, "container-2")},
						},
					))
				})

				It("is not cleaned up", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(1))
					_, prunedId := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
					Expect(prunedId).ToNot(Equal("container-2"))
				})

				It("does not release the bridge", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeBridges.ReleaseCallCount()).To(Equal(1))
				})
			})

			Context("when executing destroy.sh fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: "/root/path/destroy.sh",
						}, func(cmd *exec.Cmd) error {
							return disaster
						},
					)
				})

				It("ignores the error", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).ToNot(HaveOccurred())

					By("and does not clean up the container's rootfs")
					Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(0))
				})
			})

			It("prunes any remaining bridges", func() {
				err := pool.Prune(map[string]bool{})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeBridges.PruneCallCount()).To(Equal(1))
			})
		})
	})

	Describe("destroying", func() {
		var createdContainer *linux_container.LinuxContainer

		BeforeEach(func() {
			fakeBridges.ReserveStub = func(*net.IPNet, string) (string, error) {
				return "the-bridge", nil
			}

			container, err := pool.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			createdContainer = container.(*linux_container.LinuxContainer)

			createdContainer.Resources().AddPort(123)
			createdContainer.Resources().AddPort(456)
		})

		It("executes destroy.sh with the correct args and environment", func() {
			err := pool.Destroy(createdContainer)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/root/path/destroy.sh",
					Args: []string{path.Join(depotPath, createdContainer.ID())},
				},
			))
		})

		It("releases the container's ports, uid, and network", func() {
			err := pool.Destroy(createdContainer)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakePortPool.Released).To(ContainElement(uint32(123)))
			Expect(fakePortPool.Released).To(ContainElement(uint32(456)))

			Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))

			Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(1))
			Expect(fakeSubnetPool.ReleaseArgsForCall(0)).To(Equal(createdContainer.Resources().Network))
		})

		Describe("bridge cleanup", func() {
			It("releases the bridge from the pool", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeBridges.ReleaseCallCount()).To(Equal(1))
				bridgeName, containerId := fakeBridges.ReleaseArgsForCall(0)

				Expect(bridgeName).To(Equal("the-bridge"))
				Expect(containerId).To(Equal(createdContainer.ID()))
			})

			Context("when the releasing the bridge fails", func() {
				It("returns the error", func() {
					releaseErr := errors.New("jam in the bridge")
					fakeBridges.ReleaseReturns(releaseErr)
					err := pool.Destroy(createdContainer)
					Expect(err).To(MatchError("containerpool: release bridge the-bridge: jam in the bridge"))
				})
			})
		})

		It("tears down filter chains", func() {
			err := pool.Destroy(createdContainer)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeFilterProvider.ProvideFilterCallCount()).To(BeNumerically(">", 0))
			Expect(fakeFilterProvider.ProvideFilterArgsForCall(0)).To(Equal(createdContainer.Handle()))
			Expect(fakeFilter.TearDownCallCount()).To(Equal(1))
		})

		Context("when the container has a rootfs provider defined", func() {
			BeforeEach(func() {
				err := os.MkdirAll(path.Join(depotPath, createdContainer.ID()), 0755)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, createdContainer.ID(), "rootfs-provider"), []byte("fake"), 0644)
				Expect(err).ToNot(HaveOccurred())
			})

			It("cleans up the container's rootfs", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(1))
				_, id := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Expect(id).To(Equal(createdContainer.ID()))
			})

			Context("when cleaning up the container's rootfs fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.CleanupRootFSReturns(disaster)
				})

				It("returns the error", func() {
					err := pool.Destroy(createdContainer)
					Expect(err).To(Equal(disaster))
				})

				It("does not release the container's ports or uid", func() {
					pool.Destroy(createdContainer)

					Expect(fakePortPool.Released).ToNot(ContainElement(uint32(123)))
					Expect(fakePortPool.Released).ToNot(ContainElement(uint32(456)))
					Expect(fakeUIDPool.Released).ToNot(ContainElement(uint32(10000)))
				})

				It("does not release the network", func() {
					pool.Destroy(createdContainer)

					Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(0))
				})

				It("does not tear down the filter", func() {
					pool.Destroy(createdContainer)
					Expect(fakeFilter.TearDownCallCount()).To(Equal(0))
				})
			})
		})

		Context("when destroy.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/root/path/destroy.sh",
						Args: []string{path.Join(depotPath, createdContainer.ID())},
					},
					func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).To(Equal(disaster))
			})

			It("does not clean up the container's rootfs", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).To(HaveOccurred())

				Expect(fakeRootFSProvider.CleanupRootFSCallCount()).To(Equal(0))
			})

			It("does not release the container's ports or uid", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).To(HaveOccurred())

				Expect(fakePortPool.Released).To(BeEmpty())
				Expect(fakePortPool.Released).To(BeEmpty())
				Expect(fakeUIDPool.Released).To(BeEmpty())
			})

			It("does not release the network", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).To(HaveOccurred())
				Expect(fakeSubnetPool.ReleaseCallCount()).To(Equal(0))
			})

			It("does not tear down the filter", func() {
				pool.Destroy(createdContainer)
				Expect(fakeFilter.TearDownCallCount()).To(Equal(0))
			})
		})
	})
})
