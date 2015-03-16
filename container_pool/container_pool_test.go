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
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/bridgemgr/fake_bridge_manager"
	"github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/rootfs_provider/fake_rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/uid_pool/fake_uid_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
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
		Ω(err).ShouldNot(HaveOccurred())
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
		Ω(err).ShouldNot(HaveOccurred())

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
				Ω(pool.MaxContainers()).Should(Equal(5))
			})
		})
		Context("when constrained by uid pool size", func() {
			BeforeEach(func() {
				fakeSubnetPool.CapacityReturns(666)
				fakeUIDPool.InitialPoolSize = 42
			})

			It("returns the uid pool size", func() {
				Ω(pool.MaxContainers()).Should(Equal(42))
			})
		})
	})

	Describe("Setup", func() {
		It("executes setup.sh with the correct environment", func() {
			fakeQuotaManager.MountPointResult = "/depot/mount/point"

			err := pool.Setup()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).Should(Equal(nastyError))
			})
		})

		Describe("Setting up IPTables", func() {
			It("sets up global allow and deny rules, adding allow before deny", func() {
				err := pool.Setup()
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
					Ω(err).Should(MatchError("container_pool: setting up allow rules in iptables: oh no!"))
				})
			})
		})
	})

	Describe("creating", func() {
		itReleasesTheUserIDs := func() {
			It("returns the container's user ID and root ID to the pool", func() {
				Ω(fakeUIDPool.Released).Should(Equal([]uint32{10000, 10001}))
			})
		}

		itReleasesTheIPBlock := func() {
			It("returns the container's IP block to the pool", func() {
				Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
				Ω(fakeSubnetPool.ReleaseArgsForCall(0)).Should(Equal(containerNetwork))
			})
		}

		itDeletesTheContainerDirectory := func() {
			It("deletes the container's directory", func() {
				executedCommands := fakeRunner.ExecutedCommands()

				createCommand := executedCommands[0]
				Ω(createCommand.Path).Should(Equal("/root/path/create.sh"))
				containerPath := createCommand.Args[1]

				lastCommand := executedCommands[len(executedCommands)-1]
				Ω(lastCommand.Path).Should(Equal("/root/path/destroy.sh"))
				Ω(lastCommand.Args[1]).Should(Equal(containerPath))
			})
		}

		itCleansUpTheRootfs := func() {
			It("cleans up the rootfs for the container", func() {
				Ω(defaultFakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(1))
				_, providedID, _ := defaultFakeRootFSProvider.ProvideRootFSArgsForCall(0)
				_, cleanedUpID := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Ω(cleanedUpID).Should(Equal(providedID))
			})
		}

		itReleasesAndDestroysTheBridge := func() {
			It("releases the bridge", func() {
				Ω(fakeBridges.ReleaseCallCount()).Should(Equal(1))
				_, containerId := fakeBridges.ReserveArgsForCall(0)

				Ω(fakeBridges.ReleaseCallCount()).Should(Equal(1))
				bridgeName, containerId := fakeBridges.ReleaseArgsForCall(0)
				Ω(bridgeName).Should(Equal("bridge-for-10.2.0.0/30-" + containerId))
				Ω(containerId).Should(Equal(containerId))
			})
		}

		It("returns containers with unique IDs", func() {
			container1, err := pool.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			container2, err := pool.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container1.ID()).ShouldNot(Equal(container2.ID()))
		})

		It("creates containers with the correct grace time", func() {
			container, err := pool.Create(garden.ContainerSpec{
				GraceTime: 1 * time.Second,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.GraceTime()).Should(Equal(1 * time.Second))
		})

		It("creates containers with the correct properties", func() {
			properties := garden.Properties(map[string]string{
				"foo": "bar",
			})

			container, err := pool.Create(garden.ContainerSpec{
				Properties: properties,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.Properties()).Should(Equal(properties))
		})

		It("sets up iptable filters for the container", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeFilterProvider.ProvideFilterCallCount()).Should(BeNumerically(">", 0))
			Ω(fakeFilterProvider.ProvideFilterArgsForCall(0)).Should(Equal(container.Handle()))
			Ω(fakeFilter.SetupCallCount()).Should(Equal(1))
		})

		Context("when setting up iptables fails", func() {
			var err error
			BeforeEach(func() {
				fakeFilter.SetupReturns(errors.New("iptables says no"))
				_, err = pool.Create(garden.ContainerSpec{})
				Ω(err).Should(HaveOccurred())
			})

			It("returns a wrapped error", func() {
				Ω(err).Should(MatchError("container_pool: set up filter: iptables says no"))
			})

			itReleasesTheUserIDs()
			itReleasesTheIPBlock()
			itCleansUpTheRootfs()
			itDeletesTheContainerDirectory()
		})

		Context("when the privileged flag is specified and true", func() {
			It("executes create.sh with a root_uid of 0", func() {
				container, err := pool.Create(garden.ContainerSpec{Privileged: true})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).Should(Succeed())

				containerDir := path.Join(depotPath, container.ID())
				_, err = os.Stat(containerDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when creating the container directory fails", func() {
				JustBeforeEach(func() {
					Ω(os.Remove(depotPath)).Should(Succeed())
					ioutil.WriteFile(depotPath, []byte(""), 0755)
				})

				It("returns an error", func() {
					_, err := pool.Create(garden.ContainerSpec{})
					Ω(err).Should(MatchError(HavePrefix("containerpool: creating container directory")))
				})
			})

			Describe("allocating the requested network", func() {
				itShouldAcquire := func(subnet subnets.SubnetSelector, ip subnets.IPSelector) {
					Ω(fakeSubnetPool.AcquireCallCount()).Should(Equal(1))
					s, i := fakeSubnetPool.AcquireArgsForCall(0)

					Ω(s).Should(Equal(subnet))
					Ω(i).Should(Equal(ip))
				}

				Context("when the network string is empty", func() {
					It("allocates a dynamic subnet and ip", func() {
						_, err := pool.Create(garden.ContainerSpec{Network: ""})
						Ω(err).ShouldNot(HaveOccurred())

						itShouldAcquire(subnets.DynamicSubnetSelector, subnets.DynamicIPSelector)
					})
				})

				Context("when the network parameter is not empty", func() {
					Context("when it contains a prefix length", func() {
						It("statically allocates the requested subnet ", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0/30"})
							Ω(err).ShouldNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/30")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when it does not contain a prefix length", func() {
						It("statically allocates the requested Network from Subnets as a /30", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0"})
							Ω(err).ShouldNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/30")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when the network parameter has non-zero host bits", func() {
						It("statically allocates an IP address based on the network parameter", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.1/20"})
							Ω(err).ShouldNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/20")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.StaticIPSelector{net.ParseIP("1.2.3.1")})
						})
					})

					Context("when the network parameter has zero host bits", func() {
						It("dynamically allocates an IP address", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "1.2.3.0/24"})
							Ω(err).ShouldNot(HaveOccurred())

							_, sn, _ := net.ParseCIDR("1.2.3.0/24")
							itShouldAcquire(subnets.StaticSubnetSelector{sn}, subnets.DynamicIPSelector)
						})
					})

					Context("when an invalid network string is passed", func() {
						It("returns an error", func() {
							_, err := pool.Create(garden.ContainerSpec{Network: "not a network"})
							Ω(err).Should(MatchError("create container: invalid network spec: invalid CIDR address: not a network/30"))
						})

						It("does not acquire any resources", func() {
							Ω(fakeUIDPool.Acquired).Should(HaveLen(0))
							Ω(fakePortPool.Acquired).Should(HaveLen(0))
							Ω(fakeSubnetPool.AcquireCallCount()).Should(Equal(0))
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
					Ω(err).Should(Equal(allocateError))
				})

				itReleasesTheUserIDs()

				It("does not execute create.sh", func() {
					Ω(fakeRunner).ShouldNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/create.sh",
						},
					))
				})

				It("doesn't attempt to release the network if it has not been assigned", func() {
					Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(0))
				})
			})
		})

		It("saves the bridge name to the depot", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "bridge-name"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(string(body)).Should(Equal("bridge-for-10.2.0.0/30-" + container.ID()))
		})

		It("saves the determined rootfs provider to the depot", func() {
			container, err := pool.Create(garden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(string(body)).Should(Equal(""))
		})

		Context("when a rootfs is specified", func() {
			It("is used to provide a rootfs", func() {
				container, err := pool.Create(garden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Ω(err).ShouldNot(HaveOccurred())

				_, id, uri := fakeRootFSProvider.ProvideRootFSArgsForCall(0)
				Ω(id).Should(Equal(container.ID()))
				Ω(uri).Should(Equal(&url.URL{
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).ShouldNot(HaveOccurred())

				body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(string(body)).Should(Equal("fake"))
			})

			It("returns an error if the supplied environment is invalid", func() {
				_, err := pool.Create(garden.ContainerSpec{
					Env: []string{
						"hello",
					},
				})
				Ω(err).Should(MatchError(HavePrefix("process: malformed environment")))
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

				Ω(err).ShouldNot(HaveOccurred())
				Ω(container.(*linux_container.LinuxContainer).CurrentEnvVars()).Should(Equal(process.Env{
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
					Ω(err).Should(BeAssignableToTypeOf(&url.Error{}))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Ω(fakeBridges.ReserveCallCount()).Should(Equal(0))
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
					Ω(err).Should(Equal(container_pool.ErrUnknownRootFSProvider))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Ω(fakeBridges.ReserveCallCount()).Should(Equal(0))
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
					Ω(err).Should(Equal(providerErr))
				})

				itReleasesTheUserIDs()
				itReleasesTheIPBlock()

				It("does not acquire a bridge", func() {
					Ω(fakeBridges.ReserveCallCount()).Should(Equal(0))
				})

				It("does not execute create.sh", func() {
					Ω(fakeRunner).ShouldNot(HaveExecutedSerially(
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
				Ω(fakeRunner).ShouldNot(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
					},
				))
			})

			It("cleans up the rootfs", func() {
				Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(1))
				_, rootfsPath := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Ω(rootfsPath).Should(Equal("the-rootfs"))
			})

			It("returns an error", func() {
				Ω(err).Should(HaveOccurred())
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

				Ω(err).ShouldNot(HaveOccurred())

				containerPath := path.Join(depotPath, container.ID())
				rootfsPath := "/provided/rootfs/path"

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
					Ω(err).Should(Equal(disaster))
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
				Ω(err).Should(Equal(nastyError))
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
				Ω(err).Should(Equal(nastyError))

				Ω(fakeUIDPool.Released).Should(ContainElement(uint32(10000)))

				Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
				Ω(fakeSubnetPool.ReleaseArgsForCall(0)).Should(Equal(containerNetwork))
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
						Ω(err).ShouldNot(HaveOccurred())

						return nil
					},
				)

				_, err = pool.Create(garden.ContainerSpec{})
			})

			It("returns an error", func() {
				Ω(err).Should(HaveOccurred())
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
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("constructs a container from the snapshot", func() {
			container, err := pool.Restore(snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.ID()).Should(Equal("some-restored-id"))
			Ω(container.Handle()).Should(Equal("some-restored-handle"))
			Ω(container.GraceTime()).Should(Equal(1 * time.Second))
			Ω(container.Properties()).Should(Equal(garden.Properties(map[string]string{
				"foo": "bar",
			})))

			linuxContainer := container.(*linux_container.LinuxContainer)

			Ω(linuxContainer.State()).Should(Equal(linux_container.State("some-restored-state")))
			Ω(linuxContainer.Events()).Should(Equal([]string{
				"some-restored-event",
				"some-other-restored-event",
			}))

			Ω(linuxContainer.Resources().Network).Should(Equal(containerNetwork))
			Ω(linuxContainer.Resources().Bridge).Should(Equal("some-bridge"))
		})

		It("removes its UID from the pool", func() {
			_, err := pool.Restore(snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeUIDPool.Removed).Should(ContainElement(uint32(10000)))
		})

		Context("when the Root UID is 0", func() {
			BeforeEach(func() {
				rootUID = 0
			})

			It("does not remove it from the pool", func() {
				_, err := pool.Restore(snapshot)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeUIDPool.Removed).ShouldNot(ContainElement(rootUID))
			})
		})

		It("removes its network from the pool", func() {
			_, err := pool.Restore(snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeSubnetPool.RemoveCallCount()).Should(Equal(1))
			Ω(fakeSubnetPool.RemoveArgsForCall(0)).Should(Equal(containerNetwork))
		})

		It("removes its ports from the pool", func() {
			_, err := pool.Restore(snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakePortPool.Removed).Should(ContainElement(uint32(61001)))
			Ω(fakePortPool.Removed).Should(ContainElement(uint32(61002)))
			Ω(fakePortPool.Removed).Should(ContainElement(uint32(61003)))
		})

		It("rereserves the bridge for the subnet from the pool", func() {
			_, err := pool.Restore(snapshot)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeBridges.RereserveCallCount()).Should(Equal(1))
			bridgeName, subnet, containerId := fakeBridges.RereserveArgsForCall(0)
			Ω(bridgeName).Should(Equal("some-bridge"))
			Ω(subnet.String()).Should(Equal("2.3.4.0/29"))
			Ω(containerId).Should(Equal("some-restored-id"))
		})

		Context("when rereserving the bridge fails", func() {
			var err error

			JustBeforeEach(func() {
				fakeBridges.RereserveReturns(errors.New("boom"))
				_, err = pool.Restore(snapshot)
			})

			It("returns the error", func() {
				Ω(err).Should(HaveOccurred())
			})

			It("returns the UID to the pool", func() {
				Ω(fakeUIDPool.Released).Should(ContainElement(uint32(10000)))
			})

			It("returns the subnet to the pool", func() {
				Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
				Ω(fakeSubnetPool.ReleaseArgsForCall(0)).Should(Equal(containerNetwork))
			})
		})

		Context("when decoding the snapshot fails", func() {
			BeforeEach(func() {
				snapshot = new(bytes.Buffer)
			})

			It("fails", func() {
				_, err := pool.Restore(snapshot)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when removing the UID from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeUIDPool.RemoveError = disaster
			})

			It("returns the error", func() {
				_, err := pool.Restore(snapshot)
				Ω(err).Should(Equal(disaster))
			})
		})

		Context("when removing the network from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeSubnetPool.RemoveReturns(disaster)
			})

			It("returns the error and releases the uid", func() {
				_, err := pool.Restore(snapshot)
				Ω(err).Should(Equal(disaster))

				Ω(fakeUIDPool.Released).Should(ContainElement(uint32(10000)))
			})
		})

		Context("when removing a port from the pool fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakePortPool.RemoveError = disaster
			})

			It("returns the error and releases the uid, network, and all ports", func() {
				_, err := pool.Restore(snapshot)
				Ω(err).Should(Equal(disaster))

				Ω(fakeUIDPool.Released).Should(ContainElement(uint32(10000)))

				Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
				Ω(fakeSubnetPool.ReleaseArgsForCall(0)).Should(Equal(containerNetwork))

				Ω(fakePortPool.Released).Should(ContainElement(uint32(61001)))
				Ω(fakePortPool.Released).Should(ContainElement(uint32(61002)))
				Ω(fakePortPool.Released).Should(ContainElement(uint32(61003)))
			})

			Context("when the container is privileged", func() {
				BeforeEach(func() {
					rootUID = 0
				})

				It("does not release uid 0 back to the uid pool", func() {
					_, err := pool.Restore(snapshot)
					Ω(err).Should(Equal(disaster))

					Ω(fakeUIDPool.Released).ShouldNot(ContainElement(rootUID))
				})
			})
		})
	})

	Describe("pruning", func() {
		Context("when containers are found in the depot", func() {
			BeforeEach(func() {
				err := os.MkdirAll(path.Join(depotPath, "container-1"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "container-2"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "container-3"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.MkdirAll(path.Join(depotPath, "tmp"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-1", "bridge-name"), []byte("fake-bridge-1"), 0644)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-2", "bridge-name"), []byte("fake-bridge-2"), 0644)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-1", "rootfs-provider"), []byte("fake"), 0644)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-2", "rootfs-provider"), []byte("fake"), 0644)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, "container-3", "rootfs-provider"), []byte(""), 0644)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("destroys each container", func() {
				err := pool.Prune(map[string]bool{})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(2))
					_, id1 := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
					_, id2 := fakeRootFSProvider.CleanupRootFSArgsForCall(1)
					Ω(id1).Should(Equal("container-1"))
					Ω(id2).Should(Equal("container-2"))

					Ω(defaultFakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(1))
					_, id3 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
					Ω(id3).Should(Equal("container-3"))
				})

				It("releases the bridge", func() {
					err := pool.Prune(map[string]bool{})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeBridges.ReleaseCallCount()).Should(Equal(2))

					bridge, containerId := fakeBridges.ReleaseArgsForCall(0)
					Ω(bridge).Should(Equal("fake-bridge-1"))
					Ω(containerId).Should(Equal("container-1"))

					bridge, containerId = fakeBridges.ReleaseArgsForCall(1)
					Ω(bridge).Should(Equal("fake-bridge-2"))
					Ω(containerId).Should(Equal("container-2"))
				})
			})

			Context("when a container does not declare a bridge name", func() {
				It("does nothing much", func() {
					err := pool.Prune(map[string]bool{"container-1": true, "container-2": true})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeBridges.ReleaseCallCount()).Should(Equal(0))
				})
			})

			Context("when a container does not declare a rootfs provider", func() {
				BeforeEach(func() {
					err := os.Remove(path.Join(depotPath, "container-2", "rootfs-provider"))
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("cleans it up using the default provider", func() {
					err := pool.Prune(map[string]bool{})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(defaultFakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(2))
					_, id1 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(0)
					_, id2 := defaultFakeRootFSProvider.CleanupRootFSArgsForCall(1)
					Ω(id1).Should(Equal("container-2"))
					Ω(id2).Should(Equal("container-3"))
				})

				Context("when a container exists with an unknown rootfs provider", func() {
					BeforeEach(func() {
						err := ioutil.WriteFile(path.Join(depotPath, "container-2", "rootfs-provider"), []byte("unknown"), 0644)
						Ω(err).ShouldNot(HaveOccurred())
					})

					It("ignores the error", func() {
						err := pool.Prune(map[string]bool{})
						Ω(err).ShouldNot(HaveOccurred())
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
					Ω(err).ShouldNot(HaveOccurred())
				})
			})

			Context("when a container to keep is specified", func() {
				It("is not destroyed", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeRunner).ShouldNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/destroy.sh",
							Args: []string{path.Join(depotPath, "container-2")},
						},
					))
				})

				It("is not cleaned up", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(1))
					_, prunedId := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
					Ω(prunedId).ShouldNot(Equal("container-2"))
				})

				It("does not release the bridge", func() {
					err := pool.Prune(map[string]bool{"container-2": true})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeBridges.ReleaseCallCount()).Should(Equal(1))
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
					Ω(err).ShouldNot(HaveOccurred())

					By("and does not clean up the container's rootfs")
					Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(0))
				})
			})

			It("prunes any remaining bridges", func() {
				err := pool.Prune(map[string]bool{})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeBridges.PruneCallCount()).Should(Equal(1))
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
			Ω(err).ShouldNot(HaveOccurred())

			createdContainer = container.(*linux_container.LinuxContainer)

			createdContainer.Resources().AddPort(123)
			createdContainer.Resources().AddPort(456)
		})

		It("executes destroy.sh with the correct args and environment", func() {
			err := pool.Destroy(createdContainer)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/root/path/destroy.sh",
					Args: []string{path.Join(depotPath, createdContainer.ID())},
				},
			))
		})

		It("releases the container's ports, uid, and network", func() {
			err := pool.Destroy(createdContainer)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakePortPool.Released).Should(ContainElement(uint32(123)))
			Ω(fakePortPool.Released).Should(ContainElement(uint32(456)))

			Ω(fakeUIDPool.Released).Should(ContainElement(uint32(10000)))

			Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(1))
			Ω(fakeSubnetPool.ReleaseArgsForCall(0)).Should(Equal(createdContainer.Resources().Network))
		})

		Describe("bridge cleanup", func() {
			It("releases the bridge from the pool", func() {
				err := pool.Destroy(createdContainer)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeBridges.ReleaseCallCount()).Should(Equal(1))
				bridgeName, containerId := fakeBridges.ReleaseArgsForCall(0)

				Ω(bridgeName).Should(Equal("the-bridge"))
				Ω(containerId).Should(Equal(createdContainer.ID()))
			})

			Context("when the releasing the bridge fails", func() {
				It("returns the error", func() {
					releaseErr := errors.New("jam in the bridge")
					fakeBridges.ReleaseReturns(releaseErr)
					err := pool.Destroy(createdContainer)
					Ω(err).Should(MatchError("containerpool: release bridge the-bridge: jam in the bridge"))
				})
			})
		})

		It("tears down filter chains", func() {
			err := pool.Destroy(createdContainer)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeFilterProvider.ProvideFilterCallCount()).Should(BeNumerically(">", 0))
			Ω(fakeFilterProvider.ProvideFilterArgsForCall(0)).Should(Equal(createdContainer.Handle()))
			Ω(fakeFilter.TearDownCallCount()).Should(Equal(1))
		})

		Context("when the container has a rootfs provider defined", func() {
			BeforeEach(func() {
				err := os.MkdirAll(path.Join(depotPath, createdContainer.ID()), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(path.Join(depotPath, createdContainer.ID(), "rootfs-provider"), []byte("fake"), 0644)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("cleans up the container's rootfs", func() {
				err := pool.Destroy(createdContainer)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(1))
				_, id := fakeRootFSProvider.CleanupRootFSArgsForCall(0)
				Ω(id).Should(Equal(createdContainer.ID()))
			})

			Context("when cleaning up the container's rootfs fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.CleanupRootFSReturns(disaster)
				})

				It("returns the error", func() {
					err := pool.Destroy(createdContainer)
					Ω(err).Should(Equal(disaster))
				})

				It("does not release the container's ports or uid", func() {
					pool.Destroy(createdContainer)

					Ω(fakePortPool.Released).ShouldNot(ContainElement(uint32(123)))
					Ω(fakePortPool.Released).ShouldNot(ContainElement(uint32(456)))
					Ω(fakeUIDPool.Released).ShouldNot(ContainElement(uint32(10000)))
				})

				It("does not release the network", func() {
					pool.Destroy(createdContainer)

					Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(0))
				})

				It("does not tear down the filter", func() {
					pool.Destroy(createdContainer)
					Ω(fakeFilter.TearDownCallCount()).Should(Equal(0))
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
				Ω(err).Should(Equal(disaster))
			})

			It("does not clean up the container's rootfs", func() {
				err := pool.Destroy(createdContainer)
				Ω(err).Should(HaveOccurred())

				Ω(fakeRootFSProvider.CleanupRootFSCallCount()).Should(Equal(0))
			})

			It("does not release the container's ports or uid", func() {
				err := pool.Destroy(createdContainer)
				Ω(err).Should(HaveOccurred())

				Ω(fakePortPool.Released).Should(BeEmpty())
				Ω(fakePortPool.Released).Should(BeEmpty())
				Ω(fakeUIDPool.Released).Should(BeEmpty())
			})

			It("does not release the network", func() {
				err := pool.Destroy(createdContainer)
				Ω(err).Should(HaveOccurred())
				Ω(fakeSubnetPool.ReleaseCallCount()).Should(Equal(0))
			})

			It("does not tear down the filter", func() {
				pool.Destroy(createdContainer)
				Ω(fakeFilter.TearDownCallCount()).Should(Equal(0))
			})
		})
	})
})
