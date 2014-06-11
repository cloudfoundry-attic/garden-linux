package container_pool_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/rootfs_provider"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/rootfs_provider/fake_rootfs_provider"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network_pool/fake_network_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/uid_pool/fake_uid_pool"
	"github.com/cloudfoundry-incubator/warden-linux/sysconfig"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
)

var _ = Describe("Container pool", func() {
	var depotPath string
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var fakeUIDPool *fake_uid_pool.FakeUIDPool
	var fakeNetworkPool *fake_network_pool.FakeNetworkPool
	var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
	var fakePortPool *fake_port_pool.FakePortPool
	var defaultFakeRootFSProvider *fake_rootfs_provider.FakeRootFSProvider
	var fakeRootFSProvider *fake_rootfs_provider.FakeRootFSProvider
	var pool *container_pool.LinuxContainerPool

	BeforeEach(func() {
		_, ipNet, err := net.ParseCIDR("1.2.0.0/20")
		Expect(err).ToNot(HaveOccurred())

		fakeUIDPool = fake_uid_pool.New(10000)
		fakeNetworkPool = fake_network_pool.New(ipNet)
		fakeRunner = fake_command_runner.New()
		fakeQuotaManager = fake_quota_manager.New()
		fakePortPool = fake_port_pool.New(1000)
		defaultFakeRootFSProvider = fake_rootfs_provider.New()
		fakeRootFSProvider = fake_rootfs_provider.New()

		defaultFakeRootFSProvider.ProvideResult = "/provided/rootfs/path"

		depotPath, err = ioutil.TempDir("", "depot-path")
		Expect(err).ToNot(HaveOccurred())

		pool = container_pool.New(
			"/root/path",
			depotPath,
			sysconfig.NewConfig("0"),
			map[string]rootfs_provider.RootFSProvider{
				"":     defaultFakeRootFSProvider,
				"fake": fakeRootFSProvider,
			},
			fakeUIDPool,
			fakeNetworkPool,
			fakePortPool,
			[]string{"1.1.0.0/16", "2.2.0.0/16"},
			[]string{"1.1.1.1/32", "2.2.2.2/32"},
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
				fakeNetworkPool.InitialPoolSize = 5
				fakeUIDPool.InitialPoolSize = 3000
			})

			It("returns the network pool size", func() {
				Ω(pool.MaxContainers()).Should(Equal(5))
			})
		})
		Context("when constrained by uid pool size", func() {
			BeforeEach(func() {
				fakeNetworkPool.InitialPoolSize = 666
				fakeUIDPool.InitialPoolSize = 42
			})

			It("returns the uid pool size", func() {
				Ω(pool.MaxContainers()).Should(Equal(42))
			})
		})

	})

	Describe("setup", func() {
		It("executes setup.sh with the correct environment", func() {
			fakeQuotaManager.MountPointResult = "/depot/mount/point"

			err := pool.Setup()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/root/path/setup.sh",
					Env: []string{
						"POOL_NETWORK=1.2.0.0/20",
						"DENY_NETWORKS=1.1.0.0/16 2.2.0.0/16",
						"ALLOW_NETWORKS=1.1.1.1/32 2.2.2.2/32",
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
	})

	Describe("creating", func() {
		It("returns containers with unique IDs", func() {
			container1, err := pool.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			container2, err := pool.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			Expect(container1.ID()).ToNot(Equal(container2.ID()))
		})

		It("creates containers with the correct grace time", func() {
			container, err := pool.Create(warden.ContainerSpec{
				GraceTime: 1 * time.Second,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.GraceTime()).To(Equal(1 * time.Second))
		})

		It("creates containers with the correct properties", func() {
			properties := warden.Properties(map[string]string{
				"foo": "bar",
			})

			container, err := pool.Create(warden.ContainerSpec{
				Properties: properties,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.Properties()).To(Equal(properties))
		})

		It("executes create.sh with the correct args and environment", func() {
			container, err := pool.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/root/path/create.sh",
					Args: []string{path.Join(depotPath, container.ID())},
					Env: []string{
						"id=" + container.ID(),
						"rootfs_path=/provided/rootfs/path",
						"user_uid=10000",
						"network_host_ip=1.2.0.1",
						"network_container_ip=1.2.0.2",

						"PATH=" + os.Getenv("PATH"),
					},
				},
			))
		})

		It("saves the determined rootfs provider to the depot", func() {
			container, err := pool.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
			Expect(err).ToNot(HaveOccurred())

			Expect(string(body)).To(Equal(""))
		})

		Context("when a rootfs is specified", func() {
			It("is used to provide a rootfs", func() {
				container, err := pool.Create(warden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRootFSProvider.Provided()).To(ContainElement(fake_rootfs_provider.ProvidedSpec{
					ID: container.ID(),
					URL: &url.URL{
						Scheme: "fake",
						Host:   "",
						Path:   "/path/to/custom-rootfs",
					},
				}))
			})

			It("passes the provided rootfs as $rootfs_path to create.sh", func() {
				fakeRootFSProvider.ProvideResult = "/var/some/mount/point"

				container, err := pool.Create(warden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
						Args: []string{path.Join(depotPath, container.ID())},
						Env: []string{
							"id=" + container.ID(),
							"rootfs_path=/var/some/mount/point",
							"user_uid=10000",
							"network_host_ip=1.2.0.1",
							"network_container_ip=1.2.0.2",

							"PATH=" + os.Getenv("PATH"),
						},
					},
				))
			})

			It("saves the determined rootfs provider to the depot", func() {
				container, err := pool.Create(warden.ContainerSpec{
					RootFSPath: "fake:///path/to/custom-rootfs",
				})
				Expect(err).ToNot(HaveOccurred())

				body, err := ioutil.ReadFile(path.Join(depotPath, container.ID(), "rootfs-provider"))
				Expect(err).ToNot(HaveOccurred())

				Expect(string(body)).To(Equal("fake"))
			})

			Context("but its scheme is unknown", func() {
				It("returns ErrUnknownRootFSProvider", func() {
					_, err := pool.Create(warden.ContainerSpec{
						RootFSPath: "unknown:///path/to/custom-rootfs",
					})
					Expect(err).To(Equal(container_pool.ErrUnknownRootFSProvider))
				})
			})

			Context("when providing the mount point fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.ProvideError = disaster
				})

				It("returns the error", func() {
					_, err := pool.Create(warden.ContainerSpec{
						RootFSPath: "fake:///path/to/custom-rootfs",
					})
					Expect(err).To(Equal(disaster))
				})

				It("does not execute create.sh", func() {
					_, err := pool.Create(warden.ContainerSpec{
						RootFSPath: "fake:///path/to/custom-rootfs",
					})
					Expect(err).To(HaveOccurred())

					Expect(fakeRunner).ToNot(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: "/root/path/create.sh",
						},
					))
				})
			})
		})

		Context("when bind mounts are specified", func() {
			It("appends mount commands to hook-child-before-pivot.sh", func() {
				container, err := pool.Create(warden.ContainerSpec{
					BindMounts: []warden.BindMount{
						{
							SrcPath: "/src/path-ro",
							DstPath: "/dst/path-ro",
							Mode:    warden.BindMountModeRO,
						},
						{
							SrcPath: "/src/path-rw",
							DstPath: "/dst/path-rw",
							Mode:    warden.BindMountModeRW,
						},
						{
							SrcPath: "/src/path-rw",
							DstPath: "/dst/path-rw",
							Mode:    warden.BindMountModeRW,
							Origin:  warden.BindMountOriginContainer,
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())

				containerPath := path.Join(depotPath, container.ID())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + containerPath + "/mnt/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind /src/path-ro " + containerPath + "/mnt/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,ro /src/path-ro " + containerPath + "/mnt/dst/path-ro" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind /src/path-rw " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,rw /src/path-rw " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mkdir -p " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind " + containerPath + "/tmp/rootfs/src/path-rw " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
					fake_command_runner.CommandSpec{
						Path: "bash",
						Args: []string{
							"-c",
							"echo mount -n --bind -o remount,rw " + containerPath + "/tmp/rootfs/src/path-rw " + containerPath + "/mnt/dst/path-rw" +
								" >> " + containerPath + "/lib/hook-child-before-pivot.sh",
						},
					},
				))
			})

			Context("when appending to hook-child-before-pivot.sh fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRunner.WhenRunning(fake_command_runner.CommandSpec{
						Path: "bash",
					}, func(*exec.Cmd) error {
						return disaster
					})
				})

				It("returns the error", func() {
					_, err := pool.Create(warden.ContainerSpec{
						BindMounts: []warden.BindMount{
							{
								SrcPath: "/src/path-ro",
								DstPath: "/dst/path-ro",
								Mode:    warden.BindMountModeRO,
							},
							{
								SrcPath: "/src/path-rw",
								DstPath: "/dst/path-rw",
								Mode:    warden.BindMountModeRW,
							},
						},
					})

					Expect(err).To(Equal(disaster))
				})
			})
		})

		Context("when acquiring a UID fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeUIDPool.AcquireError = nastyError
			})

			It("returns the error", func() {
				_, err := pool.Create(warden.ContainerSpec{})
				Expect(err).To(Equal(nastyError))
			})
		})

		Context("when acquiring a network fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeNetworkPool.AcquireError = nastyError
			})

			It("returns the error and releases the uid", func() {
				_, err := pool.Create(warden.ContainerSpec{})
				Expect(err).To(Equal(nastyError))

				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))
			})
		})

		Context("when executing create.sh fails", func() {
			nastyError := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/root/path/create.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error and releases the uid and network", func() {
				_, err := pool.Create(warden.ContainerSpec{})
				Expect(err).To(Equal(nastyError))

				Expect(fakeUIDPool.Released).To(ContainElement(uint32(10000)))
				Expect(fakeNetworkPool.Released).To(ContainElement("1.2.0.0/30"))
			})
		})
	})

	Describe("restoring", func() {
		var snapshot io.Reader

		var restoredNetwork *network.Network

		BeforeEach(func() {
			buf := new(bytes.Buffer)

			snapshot = buf

			_, ipNet, err := net.ParseCIDR("10.244.0.0/30")
			Expect(err).ToNot(HaveOccurred())

			restoredNetwork = network.New(ipNet)

			err = json.NewEncoder(buf).Encode(
				linux_backend.ContainerSnapshot{
					ID:     "some-restored-id",
					Handle: "some-restored-handle",

					GraceTime: 1 * time.Second,

					State: "some-restored-state",
					Events: []string{
						"some-restored-event",
						"some-other-restored-event",
					},

					Resources: linux_backend.ResourcesSnapshot{
						UID:     10000,
						Network: restoredNetwork,
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
			Expect(container.Properties()).To(Equal(warden.Properties(map[string]string{
				"foo": "bar",
			})))

			linuxContainer := container.(*linux_backend.LinuxContainer)

			Expect(linuxContainer.State()).To(Equal(linux_backend.State("some-restored-state")))
			Expect(linuxContainer.Events()).To(Equal([]string{
				"some-restored-event",
				"some-other-restored-event",
			}))
		})

		It("removes its UID from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeUIDPool.Removed).To(ContainElement(uint32(10000)))
		})

		It("removes its network from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeNetworkPool.Removed).To(ContainElement(restoredNetwork.String()))
		})

		It("removes its ports from the pool", func() {
			_, err := pool.Restore(snapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakePortPool.Removed).To(ContainElement(uint32(61001)))
			Expect(fakePortPool.Removed).To(ContainElement(uint32(61002)))
			Expect(fakePortPool.Removed).To(ContainElement(uint32(61003)))
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
				fakeNetworkPool.RemoveError = disaster
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
				Expect(fakeNetworkPool.Released).To(ContainElement(restoredNetwork.String()))
				Expect(fakePortPool.Released).To(ContainElement(uint32(61001)))
				Expect(fakePortPool.Released).To(ContainElement(uint32(61002)))
				Expect(fakePortPool.Released).To(ContainElement(uint32(61003)))
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

					Expect(fakeRootFSProvider.CleanedUp()).To(Equal([]string{
						"container-1",
						"container-2",
					}))

					Expect(defaultFakeRootFSProvider.CleanedUp()).To(Equal([]string{
						"container-3",
					}))
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

					Expect(defaultFakeRootFSProvider.CleanedUp()).To(Equal([]string{
						"container-2",
						"container-3",
					}))
				})

				Context("when a container exists with an unknown rootfs provider", func() {
					BeforeEach(func() {
						err := ioutil.WriteFile(path.Join(depotPath, "container-2", "rootfs-provider"), []byte("unknown"), 0644)
						Expect(err).ToNot(HaveOccurred())
					})

					It("returns ErrUnknownRootFSProvider", func() {
						err := pool.Prune(map[string]bool{})
						Expect(err).To(Equal(container_pool.ErrUnknownRootFSProvider))
					})
				})
			})

			Context("when cleaning up the rootfs fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.CleanupError = disaster
				})

				It("returns the error", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).To(Equal(disaster))
				})
			})

			Context("when a container to exclude is specified", func() {
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

					Expect(fakeRootFSProvider.CleanedUp()).ToNot(ContainElement("container-2"))
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

				It("returns the error", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).To(Equal(disaster))
				})

				It("does not clean up the container's rootfs", func() {
					err := pool.Prune(map[string]bool{})
					Expect(err).To(HaveOccurred())

					Expect(fakeRootFSProvider.CleanedUp()).To(BeEmpty())
				})
			})
		})
	})

	Describe("destroying", func() {
		var createdContainer *linux_backend.LinuxContainer

		BeforeEach(func() {
			container, err := pool.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			createdContainer = container.(*linux_backend.LinuxContainer)

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

			Expect(fakeNetworkPool.Released).To(ContainElement("1.2.0.0/30"))
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

				Ω(fakeRootFSProvider.CleanedUp()).Should(ContainElement(createdContainer.ID()))
			})

			Context("when cleaning up the container's rootfs fails", func() {
				disaster := errors.New("oh no!")

				BeforeEach(func() {
					fakeRootFSProvider.CleanupError = disaster
				})

				It("returns the error", func() {
					err := pool.Destroy(createdContainer)
					Expect(err).To(Equal(disaster))
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

				Expect(fakeRootFSProvider.CleanedUp()).To(BeEmpty())
			})

			It("does not release the container's resources", func() {
				err := pool.Destroy(createdContainer)
				Expect(err).To(HaveOccurred())

				Expect(fakePortPool.Released).To(BeEmpty())
				Expect(fakePortPool.Released).To(BeEmpty())

				Expect(fakeUIDPool.Released).To(BeEmpty())

				Expect(fakeNetworkPool.Released).To(BeEmpty())
			})
		})
	})
})
