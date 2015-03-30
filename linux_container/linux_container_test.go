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
	networkFakes "github.com/cloudfoundry-incubator/garden-linux/network/fakes"
	"github.com/cloudfoundry-incubator/garden-linux/old/bandwidth_manager/fake_bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager/fake_cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/port_pool/fake_port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/quota_manager/fake_quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/process"
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
	var mtu uint32

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		fakeCgroups = fake_cgroups_manager.New("/cgroups", "some-id")

		fakeQuotaManager = fake_quota_manager.New()
		fakeBandwidthManager = fake_bandwidth_manager.New()
		fakeProcessTracker = new(fake_process_tracker.FakeProcessTracker)
		fakeFilter = new(networkFakes.FakeFilter)

		fakePortPool = fake_port_pool.New(1000)

		var err error
		containerDir, err = ioutil.TempDir("", "depot")
		Ω(err).ShouldNot(HaveOccurred())

		err = os.Mkdir(filepath.Join(containerDir, "run"), 0755)
		Ω(err).ShouldNot(HaveOccurred())
		err = ioutil.WriteFile(filepath.Join(containerDir, "run", "wshd.pid"), []byte("12345\n"), 0644)
		Ω(err).ShouldNot(HaveOccurred())

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

		mtu = 1500

		containerProps = map[string]string{
			"property-name": "property-value",
		}
	})

	JustBeforeEach(func() {
		container = linux_container.NewLinuxContainer(
			lagertest.NewTestLogger("test"),
			"some-id",
			"some-handle",
			containerDir,
			containerProps,
			1*time.Second,
			containerResources,
			fakePortPool,
			fakeRunner,
			fakeCgroups,
			fakeQuotaManager,
			fakeBandwidthManager,
			fakeProcessTracker,
			process.Env{"env1": "env1Value", "env2": "env2Value"},
			fakeFilter,
		)
	})

	It("sets the container ID", func() {
		Ω(container.ID()).Should(Equal("some-id"))
	})

	It("sets the container handle", func() {
		Ω(container.Handle()).Should(Equal("some-handle"))
	})

	It("sets the container grace time", func() {
		Ω(container.GraceTime()).Should(Equal(1 * time.Second))
	})

	Describe("Starting", func() {
		BeforeEach(func() {
			mtu = 1400
		})

		It("executes the container's start.sh with the correct environment", func() {
			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
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
			Ω(container.State()).Should(Equal(linux_container.StateBorn))

			err := container.Start()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_container.StateActive))
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
				Ω(err).Should(MatchError("container: start: oh no!"))
			})

			It("does not change the container's state", func() {
				Ω(container.State()).Should(Equal(linux_container.StateBorn))

				err := container.Start()
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_container.StateBorn))
			})
		})
	})

	Describe("Stopping", func() {
		It("executes the container's stop.sh with the appropriate arguments", func() {
			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: containerDir + "/stop.sh",
				},
			))
		})

		It("sets the container's state to stopped", func() {
			Ω(container.State()).Should(Equal(linux_container.StateBorn))

			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(container.State()).Should(Equal(linux_container.StateStopped))

		})

		Context("when kill is true", func() {
			It("executes stop.sh with -w 0", func() {
				err := container.Stop(true)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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
				Ω(err).Should(Equal(nastyError))
			})

			It("does not change the container's state", func() {
				Ω(container.State()).Should(Equal(linux_container.StateBorn))

				err := container.Stop(false)
				Ω(err).Should(HaveOccurred())

				Ω(container.State()).Should(Equal(linux_container.StateBorn))
			})
		})

		Context("when the container has an oom notifier running", func() {
			JustBeforeEach(func() {
				err := container.LimitMemory(garden.MemoryLimits{
					LimitInBytes: 42,
				})

				Ω(err).ShouldNot(HaveOccurred())
			})

			It("stops it", func() {
				err := container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveKilled(fake_command_runner.CommandSpec{
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

				Ω(err).ShouldNot(HaveOccurred())
			})

			It("stops it", func() {
				container.Cleanup()

				Ω(fakeRunner).Should(HaveKilled(fake_command_runner.CommandSpec{
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
					Ω(err).ShouldNot(HaveOccurred())

					Ω(string(bytes)).Should(Equal("the-tar-content"))

					return nil
				},
			)

			err := container.StreamIn("/some/directory/dst", bytes.NewBufferString("the-tar-content"))
			Ω(err).ShouldNot(HaveOccurred())
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
				Ω(err).Should(Equal(disaster))
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
					Ω(err).ShouldNot(HaveOccurred())

					return nil
				},
			)

			reader, err := container.StreamOut("/some/directory/dst")
			Ω(err).ShouldNot(HaveOccurred())

			bytes, err := ioutil.ReadAll(reader)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(bytes)).Should(Equal("the-compressed-content"))
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
			Ω(err).ShouldNot(HaveOccurred())

			Ω(outPipe).ShouldNot(BeNil())

			_, err = outPipe.Write([]byte("sup"))
			Ω(err).Should(HaveOccurred())
		})

		Context("when there's a trailing slash", func() {
			It("compresses the directory's contents", func() {
				_, err := container.StreamOut("/some/directory/dst/")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveBackgrounded(
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
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Net in", func() {
		It("executes net.sh in with HOST_PORT and CONTAINER_PORT", func() {
			hostPort, containerPort, err := container.NetIn(123, 456)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
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

			Ω(hostPort).Should(Equal(uint32(123)))
			Ω(containerPort).Should(Equal(uint32(456)))
		})

		Context("when a host port is not provided", func() {
			It("acquires one from the port pool", func() {
				hostPort, containerPort, err := container.NetIn(0, 456)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(hostPort).Should(Equal(uint32(1000)))
				Ω(containerPort).Should(Equal(uint32(456)))

				secondHostPort, _, err := container.NetIn(0, 456)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(secondHostPort).ShouldNot(Equal(hostPort))

				Ω(container.Resources().Ports).Should(ContainElement(hostPort))
			})

			Context("and acquiring a port from the pool fails", func() {
				disaster := errors.New("oh no!")

				JustBeforeEach(func() {
					fakePortPool.AcquireError = disaster
				})

				It("returns the error", func() {
					_, _, err := container.NetIn(0, 456)
					Ω(err).Should(Equal(disaster))
				})
			})
		})

		Context("when a container port is not provided", func() {
			It("defaults it to the host port", func() {
				hostPort, containerPort, err := container.NetIn(123, 0)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRunner).Should(HaveExecutedSerially(
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

				Ω(hostPort).Should(Equal(uint32(123)))
				Ω(containerPort).Should(Equal(uint32(123)))
			})

			Context("and a host port is not provided either", func() {
				It("defaults it to the same acquired port", func() {
					hostPort, containerPort, err := container.NetIn(0, 0)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeRunner).Should(HaveExecutedSerially(
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

					Ω(hostPort).Should(Equal(uint32(1000)))
					Ω(containerPort).Should(Equal(uint32(1000)))
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
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Net out", func() {
		It("delegates to the filter", func() {
			rule := garden.NetOutRule{}
			err := container.NetOut(rule)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeFilter.NetOutCallCount()).Should(Equal(1))
			passedRule := fakeFilter.NetOutArgsForCall(0)
			Ω(passedRule).Should(Equal(rule))
		})

		Context("when the filter fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeFilter.NetOutReturns(disaster)
			})

			It("returns the error", func() {
				err := container.NetOut(garden.NetOutRule{})
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("Properties", func() {
		Describe("CRUD", func() {
			It("can get a property", func() {
				value, err := container.GetProperty("property-name")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("property-value"))
			})

			It("can test for a set of properties", func() {
				Ω(container.HasProperties(garden.Properties{
					"other-property": "property-value",
				})).Should(BeFalse())

				Ω(container.HasProperties(garden.Properties{
					"property-name":  "property-value",
					"other-property": "property-value",
				})).Should(BeFalse())

				Ω(container.HasProperties(garden.Properties{
					"property-name": "property-value",
				})).Should(BeTrue())
			})

			It("returns an error when the property is undefined", func() {
				_, err := container.GetProperty("some-other-property")
				Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"some-other-property"}))
				Ω(err).Should(MatchError("property does not exist: some-other-property"))
			})

			It("can set a new property", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Ω(err).ShouldNot(HaveOccurred())
				value, err := container.GetProperty("some-other-property")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-value"))
			})

			It("can override an existing property", func() {
				err := container.SetProperty("property-name", "some-other-new-value")
				Ω(err).ShouldNot(HaveOccurred())

				value, err := container.GetProperty("property-name")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-new-value"))
			})

			Context("when removing a property", func() {
				var err error

				JustBeforeEach(func() {
					err = container.SetProperty("other-property-name", "some-other-value")
					Ω(err).ShouldNot(HaveOccurred())

					err = container.RemoveProperty("property-name")
				})

				It("removes the property", func() {
					Ω(err).ShouldNot(HaveOccurred())

					_, err = container.GetProperty("property-name")
					Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"property-name"}))
				})

				It("does not remove other properties", func() {
					value, err := container.GetProperty("other-property-name")
					Ω(err).ShouldNot(HaveOccurred())
					Ω(value).Should(Equal("some-other-value"))
				})
			})

			It("returns an error when removing an undefined property", func() {
				err := container.RemoveProperty("some-other-property")
				Ω(err).Should(HaveOccurred())
				Ω(err).Should(MatchError("property does not exist: some-other-property"))
			})
		})

		It("can return all properties as a map", func() {
			properties, err := container.GetProperties()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(properties).Should(Equal(garden.Properties{"property-name": "property-value"}))
		})

		It("returns a properties snapshot", func() {
			err := container.SetProperty("some-property", "some-value")
			Ω(err).ShouldNot(HaveOccurred())

			properties := container.Properties()
			Ω(properties["some-property"]).Should(Equal("some-value"))

			err = container.SetProperty("some-property", "some-other-value")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(properties["some-property"]).Should(Equal("some-value"))
		})

		Context("with a nil map of properties at container creation", func() {
			BeforeEach(func() {
				containerProps = nil
			})

			It("reading a property fails in the expected way", func() {
				_, err := container.GetProperty("property-name")
				Ω(err).Should(Equal(linux_container.UndefinedPropertyError{"property-name"}))
			})

			It("setting a property succeeds", func() {
				err := container.SetProperty("some-other-property", "some-other-value")
				Ω(err).ShouldNot(HaveOccurred())

				value, err := container.GetProperty("some-other-property")
				Ω(err).ShouldNot(HaveOccurred())
				Ω(value).Should(Equal("some-other-value"))
			})
		})
	})

	Describe("Info", func() {
		It("returns the container's state", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("born"))
		})

		It("returns the container's events", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Events).Should(Equal([]string{}))
		})

		It("returns the container's properties", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties).Should(Equal(container.Properties()))
		})

		It("returns the container's network info", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.HostIP).Should(Equal("2.3.4.2"))
			Ω(info.ContainerIP).Should(Equal("1.2.3.4"))
		})

		It("returns the container's path", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(info.ContainerPath).Should(Equal(containerDir))
		})

		It("returns the container's mapped ports", func() {
			_, _, err := container.NetIn(1234, 5678)
			Ω(err).ShouldNot(HaveOccurred())

			_, _, err = container.NetIn(1235, 5679)
			Ω(err).ShouldNot(HaveOccurred())

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(info.MappedPorts).Should(Equal([]garden.PortMapping{
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
				Ω(err).ShouldNot(HaveOccurred())
				Ω(info.ProcessIDs).Should(Equal([]uint32{1, 2, 3}))
			})
		})
	})
})
