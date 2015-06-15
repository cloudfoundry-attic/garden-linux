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
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/fake_quota_manager"
	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/old/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker/fake_process_tracker"
	wfakes "github.com/cloudfoundry-incubator/garden/fakes"
	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
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
	var containerDir string
	var containerProps map[string]string

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = new(fake_quota_manager.FakeQuotaManager)
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)
		fakeFilter = new(networkFakes.FakeFilter)

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
			1234,
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
			lagertest.NewTestLogger("linux-container-limits-test"),
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

				Expect(fakeRunner).To(HaveKilled(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}))

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

				Expect(fakeRunner).To(HaveKilled(fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/oom",
				}))

			})
		})
	})

	Describe("Streaming data in", func() {
		It("streams the input to tar xf in the container", func() {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory/dst",
					},
				},
				func(cmd *exec.Cmd) error {
					bytes, err := ioutil.ReadAll(cmd.Stdin)
					Expect(err).ToNot(HaveOccurred())

					Expect(string(bytes)).To(Equal("the-tar-content"))

					return nil
				},
			)

			err := container.StreamIn("/some/directory/dst", bytes.NewBufferString("the-tar-content"))
			Expect(err).ToNot(HaveOccurred())
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
				err := container.StreamIn("/some/directory/dst", nil)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("Streaming out", func() {
		It("streams the output of tar cf to the destination", func() {
			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory",
						"dst",
					},
				},
				func(cmd *exec.Cmd) error {
					_, err := cmd.Stdout.Write([]byte("the-compressed-content"))
					Expect(err).ToNot(HaveOccurred())

					return nil
				},
			)

			reader, err := container.StreamOut("/some/directory/dst")
			Expect(err).ToNot(HaveOccurred())

			bytes, err := ioutil.ReadAll(reader)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(bytes)).To(Equal("the-compressed-content"))
		})

		It("closes the server-side dupe of of the pipe's write end", func() {
			var outPipe io.Writer

			fakeRunner.WhenRunning(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/bin/nstar",
					Args: []string{
						"12345",
						"vcap",
						"/some/directory",
						"dst",
					},
				},
				func(cmd *exec.Cmd) error {
					outPipe = cmd.Stdout
					return nil
				},
			)

			_, err := container.StreamOut("/some/directory/dst")
			Expect(err).ToNot(HaveOccurred())

			Expect(outPipe).ToNot(BeNil())

			_, err = outPipe.Write([]byte("sup"))
			Expect(err).To(HaveOccurred())
		})

		Context("when there's a trailing slash", func() {
			It("compresses the directory's contents", func() {
				_, err := container.StreamOut("/some/directory/dst/")
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRunner).To(HaveBackgrounded(
					fake_command_runner.CommandSpec{
						Path: containerDir + "/bin/nstar",
						Args: []string{
							"12345",
							"vcap",
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
				_, err := container.StreamOut("/some/dst")
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

			Expect(info.HostIP).To(Equal("2.3.4.2"))
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

		Context("with running processes", func() {
			JustBeforeEach(func() {
				p1 := new(wfakes.FakeProcess)
				p1.IDReturns(1)

				p2 := new(wfakes.FakeProcess)
				p2.IDReturns(2)

				p3 := new(wfakes.FakeProcess)
				p3.IDReturns(3)

				fakeProcessTracker.ActiveProcessesReturns([]garden.Process{p1, p2, p3})
			})

			It("returns their process IDs", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())
				Expect(info.ProcessIDs).To(Equal([]uint32{1, 2, 3}))
			})
		})
	})
})
