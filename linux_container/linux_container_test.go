package linux_container_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	"fmt"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-linux/linux_backend"
	"code.cloudfoundry.org/garden-linux/linux_container"
	"code.cloudfoundry.org/garden-linux/linux_container/bandwidth_manager/fake_bandwidth_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/cgroups_manager/fake_cgroups_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_iptables_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_network_statisticser"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_quota_manager"
	"code.cloudfoundry.org/garden-linux/linux_container/fake_watcher"
	networkFakes "code.cloudfoundry.org/garden-linux/network/fakes"
	"code.cloudfoundry.org/garden-linux/port_pool/fake_port_pool"
	"code.cloudfoundry.org/garden-linux/process_tracker/fake_process_tracker"
	wfakes "code.cloudfoundry.org/garden/gardenfakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

var _ = Describe("Linux containers", func() {
	var fakeCgroups *fake_cgroups_manager.FakeCgroupsManager
	var fakeQuotaManager *fake_quota_manager.FakeQuotaManager
	var fakeBandwidthManager *fake_bandwidth_manager.FakeBandwidthManager
	var fakeRunner *fake_command_runner.FakeCommandRunner
	var containerResources *linux_backend.Resources
	var container *linux_container.LinuxContainer
	var fakePortPool *fake_port_pool.FakePortPool
	var fakeProcessTracker *fake_process_tracker.FakeProcessTracker
	var fakeFilter *networkFakes.FakeFilter
	var fakeIPTablesManager *fake_iptables_manager.FakeIPTablesManager
	var fakeOomWatcher *fake_watcher.FakeWatcher
	var containerDir string
	var containerProps map[string]string
	var logger *lagertest.TestLogger

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = new(fake_quota_manager.FakeQuotaManager)
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)
		fakeFilter = new(networkFakes.FakeFilter)
		fakeIPTablesManager = new(fake_iptables_manager.FakeIPTablesManager)
		fakeOomWatcher = new(fake_watcher.FakeWatcher)

		fakePortPool = fake_port_pool.New(1000)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Expect(err).ToNot(HaveOccurred())

		err = os.Mkdir(filepath.Join(containerDir, "run"), 0755)
		Expect(err).ToNot(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(containerDir, "run", "wshd.pid"), []byte("12345\n"), 0644)
		Expect(err).ToNot(HaveOccurred())

		_, subnet, err := net.ParseCIDR("2.3.4.0/30")
		containerResources = linux_backend.NewResources(
			1235,
			&linux_backend.Network{
				IP:     net.ParseIP("1.2.3.4"),
				Subnet: subnet,
			},
			"some-bridge",
			[]uint32{},
			nil,
		)

		containerProps = map[string]string{
			"property-name": "property-value",
		}

		logger = lagertest.NewTestLogger("linux-container")
	})

	JustBeforeEach(func() {
		container = linux_container.NewLinuxContainer(
			linux_backend.LinuxContainerSpec{
				ID:                  "some-id",
				ContainerPath:       containerDir,
				ContainerRootFSPath: "some-volume-path",
				Resources:           containerResources,
				State:               linux_backend.StateBorn,
				ContainerSpec: garden.ContainerSpec{
					Handle:     "some-handle",
					GraceTime:  time.Second * 1,
					Properties: containerProps,
				},
			},
			fakePortPool,
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			fakeProcessTracker,
			fakeFilter,
			fakeIPTablesManager,
			new(fake_network_statisticser.FakeNetworkStatisticser),
			fakeOomWatcher,
			logger,
		)
	})

	It("sets the container ID", func() {
		Expect(container.ID()).To(Equal("some-id"))
	})

	It("sets the container handle", func() {
		Expect(container.Handle()).To(Equal("some-handle"))
	})

	It("sets the container subvolume path", func() {
		Expect(container.RootFSPath()).To(Equal("some-volume-path"))
	})

	It("sets the container grace time", func() {
		Expect(container.GraceTime()).To(Equal(1 * time.Second))
	})

	Describe("Starting", func() {
		It("should setup IPTables", func() {
			err := container.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeIPTablesManager.ContainerSetupCallCount()).To(Equal(1))
			id, bridgeIface, ip, network := fakeIPTablesManager.ContainerSetupArgsForCall(0)
			Expect(id).To(Equal("some-id"))
			Expect(bridgeIface).To(Equal("some-bridge"))
			Expect(err).ToNot(HaveOccurred())
			Expect(ip).To(Equal(containerResources.Network.IP))
			Expect(network).To(Equal(containerResources.Network.Subnet))
		})

		Context("when IPTables setup fails", func() {
			JustBeforeEach(func() {
				fakeIPTablesManager.ContainerSetupReturns(errors.New("oh yes!"))
			})

			It("should return a wrapped error", func() {
				Expect(container.Start()).To(MatchError("container: start: oh yes!"))
			})

			It("should not call start.sh", func() {
				Expect(container.Start()).ToNot(Succeed())

				Expect(fakeRunner).ToNot(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/start.sh",
					},
				))
			})
		})

		It("executes the container's start.sh with the correct environment", func() {
			err := container.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/start.sh",
					Env: []string{
						"id=some-id",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))
		})

		It("changes the container's state to active", func() {
			Expect(container.State()).To(Equal(linux_backend.StateBorn))

			err := container.Start()
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.StateActive))
		})

		It("should log before and after", func() {
			Expect(container.Start()).To(Succeed())

			logs := logger.Logs()
			expectedData := lager.Data{"handle": "some-handle"}
			Expect(logs).To(ContainLogWithData("linux-container.start.iptables-setup-starting", expectedData))
			Expect(logs).To(ContainLogWithData("linux-container.start.iptables-setup-ended", expectedData))
			Expect(logs).To(ContainLogWithData("linux-container.start.wshd-start-starting", expectedData))
			Expect(logs).To(ContainLogWithData("linux-container.start.wshd-start-ended", expectedData))
		})

		Context("when start.sh fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/start.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns a wrapped error", func() {
				err := container.Start()
				Expect(err).To(MatchError("container: start: oh no!"))
			})

			It("does not change the container's state", func() {
				Expect(container.State()).To(Equal(linux_backend.StateBorn))

				err := container.Start()
				Expect(err).To(HaveOccurred())

				Expect(container.State()).To(Equal(linux_backend.StateBorn))
			})
		})
	})

	Describe("Stopping", func() {
		It("executes the container's stop.sh with the appropriate arguments", func() {
			err := container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/stop.sh",
				},
			))
		})

		It("sets the container's state to stopped", func() {
			Expect(container.State()).To(Equal(linux_backend.StateBorn))

			err := container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			Expect(container.State()).To(Equal(linux_backend.StateStopped))

		})

		Context("when kill is true", func() {
			It("executes stop.sh with -w 0", func() {
				err := container.Stop(true)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
						Args: []string{"-w", "0"},
					},
				))

			})
		})

		Context("when stop.sh fails", func() {
			nastyError := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/stop.sh",
					}, func(*exec.Cmd) error {
						return nastyError
					},
				)
			})

			It("returns the error", func() {
				err := container.Stop(false)
				Expect(err).To(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Expect(container.State()).To(Equal(linux_backend.StateBorn))

				err := container.Stop(false)
				Expect(err).To(HaveOccurred())

				Expect(container.State()).To(Equal(linux_backend.StateBorn))
			})
		})

		Context("when the container has an oom notifier running", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 42,
				})

				Expect(err).ToNot(HaveOccurred())
			})

			It("stops it", func() {
				err := container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeOomWatcher.UnwatchCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Cleaning up", func() {
		Context("when the container has an oom notifier running", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 42,
				})

				Expect(err).ToNot(HaveOccurred())
			})

			It("stops it", func() {
				container.Cleanup()
				Expect(fakeOomWatcher.UnwatchCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Streaming data in", func() {
		It("streams the input to tar xf in the container as the specified user", func() {
			cmdSpec := fake_command_runner.CommandSpec{
				Path: containerDir + "/bin/nstar",
				Args: []string{
					containerDir + "/bin/tar",
					"12345",
					"bob",
					"/some/directory/dst",
				},
			}
			fakeRunner.WhenRunning(
				cmdSpec,
				func(cmd *exec.Cmd) error {
					bytes, err := ioutil.ReadAll(cmd.Stdin)
					Expect(err).ToNot(HaveOccurred())

					Expect(string(bytes)).To(Equal("the-tar-content"))

					return nil
				},
			)

			err := container.StreamIn(garden.StreamInSpec{
				User:      "bob",
				Path:      "/some/directory/dst",
				TarStream: bytes.NewBufferString("the-tar-content"),
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(cmdSpec))
		})

		Context("when no user specified", func() {
			It("streams the input to tar as root", func() {
				cmdSpec := fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						containerDir + "/bin/tar",
						"12345",
						"root",
						"/some/directory/dst",
					},
				}
				fakeRunner.WhenRunning(
					cmdSpec,
					func(cmd *exec.Cmd) error {
						bytes, err := ioutil.ReadAll(cmd.Stdin)
						Expect(err).ToNot(HaveOccurred())

						Expect(string(bytes)).To(Equal("the-tar-content"))

						return nil
					},
				)

				err := container.StreamIn(garden.StreamInSpec{
					Path:      "/some/directory/dst",
					TarStream: bytes.NewBufferString("the-tar-content"),
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(cmdSpec))
			})
		})

		Context("when tar fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
					},
					func(cmd *exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := container.StreamIn(garden.StreamInSpec{
					User: "alice",
					Path: "/some/directory/dst",
				})
				Expect(err).To(MatchError(ContainSubstring("oh no!")))
			})
		})
	})

	Describe("Streaming out", func() {
		It("streams the output of tar cf to the destination as the specified user", func() {
			cmdSpec := fake_command_runner.CommandSpec{
				Path: containerDir + "/bin/nstar",
				Args: []string{
					containerDir + "/bin/tar",
					"12345",
					"alice",
					"/some/directory",
					"dst",
				},
			}
			fakeRunner.WhenRunning(
				cmdSpec,
				func(cmd *exec.Cmd) error {
					_, err := cmd.Stdout.Write([]byte("the-compressed-content"))
					Expect(err).ToNot(HaveOccurred())

					return nil
				},
			)

			reader, err := container.StreamOut(garden.StreamOutSpec{
				User: "alice",
				Path: "/some/directory/dst",
			})
			Expect(err).ToNot(HaveOccurred())

			bytes, err := ioutil.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(bytes)).To(Equal("the-compressed-content"))

			Expect(fakeRunner).To(HaveBackgrounded(cmdSpec))
		})

		Context("when no user specified", func() {
			It("streams the output of tar as root", func() {
				cmdSpec := fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						containerDir + "/bin/tar",
						"12345",
						"root",
						"/some/directory",
						"dst",
					},
				}
				fakeRunner.WhenRunning(
					cmdSpec,
					func(cmd *exec.Cmd) error {
						_, err := cmd.Stdout.Write([]byte("the-compressed-content"))
						Expect(err).ToNot(HaveOccurred())

						return nil
					},
				)

				reader, err := container.StreamOut(garden.StreamOutSpec{
					Path: "/some/directory/dst",
				})
				Expect(err).ToNot(HaveOccurred())

				bytes, err := ioutil.ReadAll(reader)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(bytes)).To(Equal("the-compressed-content"))

				Expect(fakeRunner).To(HaveBackgrounded(cmdSpec))
			})
		})

		It("closes the server-side dupe of of the pipe's write end", func() {
			var outPipe io.Writer

			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						containerDir + "/bin/tar",
						"12345",
						"herbert",
						"/some/directory",
						"dst",
					},
				},
				func(cmd *exec.Cmd) error {
					outPipe = cmd.Stdout
					return nil
				},
			)

			_, err := container.StreamOut(garden.StreamOutSpec{
				User: "herbert",
				Path: "/some/directory/dst",
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(outPipe).ToNot(BeNil())

			_, err = outPipe.Write([]byte("sup"))
			Expect(err).To(HaveOccurred())
		})

		Context("when there's a trailing slash", func() {
			It("compresses the directory's contents", func() {
				_, err := container.StreamOut(garden.StreamOutSpec{
					User: "alice",
					Path: "/some/directory/dst/",
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveBackgrounded(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
						Args: []string{
							containerDir + "/bin/tar",
							"12345",
							"alice",
							"/some/directory/dst/",
							".",
						},
					},
				))
			})
		})

		Context("when executing the command fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, err := container.StreamOut(garden.StreamOutSpec{User: "alice", Path: "/some/dst"})
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Net in", func() {
		It("executes net.sh in with HOST_PORT and CONTAINER_PORT", func() {
			hostPort, containerPort, err := container.NetIn(123, 456)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/net.sh",
					Args: []string{"in"},
					Env: []string{
						"HOST_PORT=123",
						"CONTAINER_PORT=456",
						"PATH=" + os.Getenv("PATH"),
					},
				},
			))

			Expect(hostPort).To(Equal(uint32(123)))
			Expect(containerPort).To(Equal(uint32(456)))
		})

		Context("when a host port is not provided", func() {
			It("acquires one from the port pool", func() {
				hostPort, containerPort, err := container.NetIn(0, 456)
				Expect(err).ToNot(HaveOccurred())

				Expect(hostPort).To(Equal(uint32(1000)))
				Expect(containerPort).To(Equal(uint32(456)))

				secondHostPort, _, err := container.NetIn(0, 456)
				Expect(err).ToNot(HaveOccurred())

				Expect(secondHostPort).ToNot(Equal(hostPort))

				Expect(container.Resources.Ports).To(ContainElement(hostPort))
			})

			Context("and acquiring a port from the pool fails", func() {
				disaster := errors.New("oh no!")

				JustBeforeEach(func() {
					fakePortPool.AcquireError = disaster
				})

				It("returns the error", func() {
					_, _, err := container.NetIn(0, 456)
					Expect(err).To(Equal(disaster))
				})
			})
		})

		Context("when a container port is not provided", func() {
			It("defaults it to the host port", func() {
				hostPort, containerPort, err := container.NetIn(123, 0)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
						Args: []string{"in"},
						Env: []string{
							"HOST_PORT=123",
							"CONTAINER_PORT=123",
							"PATH=" + os.Getenv("PATH"),
						},
					},
				))

				Expect(hostPort).To(Equal(uint32(123)))
				Expect(containerPort).To(Equal(uint32(123)))
			})

			Context("and a host port is not provided either", func() {
				It("defaults it to the same acquired port", func() {
					hostPort, containerPort, err := container.NetIn(0, 0)
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRunner).To(HaveExecutedSerially(
						fake_command_runner.CommandSpec{
							Path: containerDir + "/net.sh",
							Args: []string{"in"},
							Env: []string{
								"HOST_PORT=1000",
								"CONTAINER_PORT=1000",
								"PATH=" + os.Getenv("PATH"),
							},
						},
					))

					Expect(hostPort).To(Equal(uint32(1000)))
					Expect(containerPort).To(Equal(uint32(1000)))
				})
			})
		})

		Context("when net.sh fails", func() {
			disaster := errors.New("oh no!")

			JustBeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/net.sh",
					}, func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				_, _, err := container.NetIn(123, 456)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Net out", func() {
		It("delegates to the filter", func() {
			rule := garden.NetOutRule{}
			err := container.NetOut(rule)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeFilter.NetOutCallCount()).To(Equal(1))
			passedRule := fakeFilter.NetOutArgsForCall(0)
			Expect(passedRule).To(Equal(rule))
		})

		Context("when the filter fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeFilter.NetOutReturns(disaster)
			})

			It("returns the error", func() {
				err := container.NetOut(garden.NetOutRule{})
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Properties", func() {
		Describe("CRUD", func() {
			It("can get a property", func() {
				value, err := container.Property("property-name")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("property-value"))
			})

			It("can test for a set of properties", func() {
				Expect(container.HasProperties(garden.Properties{
					"other-property": "property-value",
				})).To(BeFalse())

				Expect(container.HasProperties(garden.Properties{
					"property-name":  "property-value",
					"other-property": "property-value",
				})).To(BeFalse())

				Expect(container.HasProperties(garden.Properties{
					"property-name": "property-value",
				})).To(BeTrue())
			})

			It("returns an error when the property is undefined", func() {
				_, err := container.Property("some-other-property")
				Expect(err).To(Equal(linux_container.UndefinedPropertyError{"some-other-property"}))
				Expect(err).To(MatchError("property does not exist: some-other-property"))
			})

			It("can set a new property", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Expect(err).ToNot(HaveOccurred())
				value, err := container.Property("some-other-property")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("some-other-value"))
			})

			It("can override an existing property", func() {
				err := container.SetProperty("property-name", "some-other-new-value")
				Expect(err).ToNot(HaveOccurred())

				value, err := container.Property("property-name")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("some-other-new-value"))
			})

			Context("when removing a property", func() {
				var err error

				JustBeforeEach(func() {
					err = container.SetProperty("other-property-name", "some-other-value")
					Expect(err).ToNot(HaveOccurred())

					err = container.RemoveProperty("property-name")
				})

				It("removes the property", func() {
					Expect(err).ToNot(HaveOccurred())

					_, err = container.Property("property-name")
					Expect(err).To(Equal(linux_container.UndefinedPropertyError{"property-name"}))
				})

				It("does not remove other properties", func() {
					value, err := container.Property("other-property-name")
					Expect(err).ToNot(HaveOccurred())
					Expect(value).To(Equal("some-other-value"))
				})
			})

			It("returns an error when removing an undefined property", func() {
				err := container.RemoveProperty("some-other-property")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("property does not exist: some-other-property"))
			})
		})

		It("can return all properties as a map", func() {
			properties, err := container.Properties()
			Expect(err).ToNot(HaveOccurred())

			Expect(properties).To(Equal(garden.Properties{"property-name": "property-value"}))
		})

		It("returns a properties snapshot", func() {
			err := container.SetProperty("some-property", "some-value")
			Expect(err).ToNot(HaveOccurred())

			properties, err := container.Properties()
			Expect(err).ToNot(HaveOccurred())
			Expect(properties["some-property"]).To(Equal("some-value"))

			err = container.SetProperty("some-property", "some-other-value")
			Expect(err).ToNot(HaveOccurred())

			Expect(properties["some-property"]).To(Equal("some-value"))
		})

		Context("with a nil map of properties at container creation", func() {
			BeforeEach(func() {
				containerProps = nil
			})

			It("reading a property fails in the expected way", func() {
				_, err := container.Property("property-name")
				Expect(err).To(Equal(linux_container.UndefinedPropertyError{"property-name"}))
			})

			It("setting a property succeeds", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Expect(err).ToNot(HaveOccurred())

				value, err := container.Property("some-other-property")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("some-other-value"))
			})
		})
	})

	Describe("GraceTime", func() {
		Context("when SetGraceTime is called", func() {
			var newGraceTime time.Duration

			JustBeforeEach(func() {
				newGraceTime = 12 * time.Minute

				Expect(container.SetGraceTime(newGraceTime)).To(Succeed())
			})

			It("sets grace time", func() {
				Expect(container.GraceTime()).To(Equal(newGraceTime))
			})
		})
	})

	Describe("Info", func() {
		It("returns the container's state", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("born"))
		})

		It("returns the container's events", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Events).To(Equal([]string{}))
		})

		It("returns the container's properties", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			properties, err := container.Properties()
			Expect(info.Properties).To(Equal(properties))
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns the container's network info", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.HostIP).To(Equal("2.3.4.1"))
			Expect(info.ContainerIP).To(Equal("1.2.3.4"))
		})

		It("returns the container's path", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())
			Expect(info.ContainerPath).To(Equal(containerDir))
		})

		It("returns the container's mapped ports", func() {
			_, _, err := container.NetIn(1234, 5678)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(1235, 5679)
			Expect(err).ToNot(HaveOccurred())

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())
			Expect(info.MappedPorts).To(Equal([]garden.PortMapping{
				{HostPort: 1234, ContainerPort: 5678},
				{HostPort: 1235, ContainerPort: 5679},
			}))

		})

		It("should log before and after", func() {
			_, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(logger.LogMessages()).To(ContainElement(ContainSubstring("info-starting")))
			Expect(logger.LogMessages()).To(ContainElement(ContainSubstring("info-ended")))
		})

		Context("with running processes", func() {
			JustBeforeEach(func() {
				p1 := new(wfakes.FakeProcess)
				p1.IDReturns("1")

				p2 := new(wfakes.FakeProcess)
				p2.IDReturns("2")

				p3 := new(wfakes.FakeProcess)
				p3.IDReturns("3")

				fakeProcessTracker.ActiveProcessesReturns([]garden.Process{p1, p2, p3})
			})

			It("returns their process IDs", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())
				Expect(info.ProcessIDs).To(Equal([]string{"1", "2", "3"}))
			})
		})
	})
})

func ContainLogWithData(message string, data lager.Data) types.GomegaMatcher {
	return &containLogMatcher{
		message: message,
		data:    data,
	}
}

type containLogMatcher struct {
	message string
	data    lager.Data
}

func (m *containLogMatcher) Match(actual interface{}) (success bool, err error) {
	logs, ok := actual.([]lager.LogFormat)
	if !ok {
		return false, errors.New("invalid type")
	}

	for _, log := range logs {
		if log.Message == m.message {
			for k, v := range m.data {
				if value, ok := log.Data[k]; !ok || value != v {
					return false, nil
				}
			}

			return true, nil
		}
	}

	return false, nil
}

func (m *containLogMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain log matching", m.String())
}

func (m *containLogMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to contain log matching", m.String())
}

func (m *containLogMatcher) String() string {
	return fmt.Sprintf("message %s, data %v", m.message, m.data)
}
