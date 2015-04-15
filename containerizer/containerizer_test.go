package containerizer_test

import (
	"errors"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_container_execer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_rootfs_enterer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_set_uider"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_signaller"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_waiter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Containerizer", func() {
	Describe("Create", func() {
		var cz *containerizer.Containerizer
		var containerExecer *fake_container_execer.FakeContainerExecer
		var signaller *fake_signaller.FakeSignaller
		var waiter *fake_waiter.FakeWaiter

		BeforeEach(func() {
			containerExecer = &fake_container_execer.FakeContainerExecer{}
			signaller = &fake_signaller.FakeSignaller{}
			waiter = &fake_waiter.FakeWaiter{}

			cz = &containerizer.Containerizer{
				Execer:      containerExecer,
				InitBinPath: "initd",
				Signaller:   signaller,
				Waiter:      waiter,
			}
		})

		It("Runs the initd process in a container", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(containerExecer.ExecCallCount()).To(Equal(1))
			binPath, args := containerExecer.ExecArgsForCall(0)
			Expect(binPath).To(Equal("initd"))
			Expect(args).To(BeEmpty())
		})

		Context("when execer fails", func() {
			BeforeEach(func() {
				containerExecer.ExecReturns(0, errors.New("Oh my gawsh"))
			})

			It("returns an error", func() {
				Expect(cz.Create()).To(MatchError("containerizer: Failed to create container: Oh my gawsh"))
			})

			It("does not signal the container", func() {
				cz.Create()
				Expect(signaller.SignalSuccessCallCount()).To(Equal(0))
			})
		})

		PIt("exports PID environment variable", func() {})

		It("sends signal to container", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(signaller.SignalSuccessCallCount()).To(Equal(1))
		})

		Context("when the signaller fails", func() {
			BeforeEach(func() {
				signaller.SignalSuccessReturns(errors.New("Dooo"))
			})

			It("returns an error", func() {
				Expect(cz.Create()).To(MatchError("containerizer: Failed to send success singnal to the container: Dooo"))
			})

			It("does not wait for the container", func() {
				cz.Create()
				Expect(waiter.WaitCallCount()).To(Equal(0))
			})
		})

		It("waits for container", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(waiter.WaitCallCount()).To(Equal(1))
		})

		Context("when the waiter fails", func() {
			BeforeEach(func() {
				waiter.WaitReturns(errors.New("Foo"))
			})

			It("returns an error", func() {
				Expect(cz.Create()).To(MatchError("containerizer: Failed to wait for container: Foo"))
			})
		})
	})

	Describe("Run", func() {
		var cz *containerizer.Containerizer
		var rootFS *fake_rootfs_enterer.FakeRootFSEnterer
		var setUider *fake_set_uider.FakeSetUider
		var daemon *fake_container_daemon.FakeContainerDaemon
		var signaller *fake_signaller.FakeSignaller
		var waiter *fake_waiter.FakeWaiter
		var workingDirectory string

		BeforeEach(func() {
			var err error

			workingDirectory, err = os.Getwd()
			Expect(err).ToNot(HaveOccurred())

			rootFS = &fake_rootfs_enterer.FakeRootFSEnterer{}
			setUider = &fake_set_uider.FakeSetUider{}
			daemon = &fake_container_daemon.FakeContainerDaemon{}
			signaller = &fake_signaller.FakeSignaller{}
			waiter = &fake_waiter.FakeWaiter{}

			cz = &containerizer.Containerizer{
				RootFS:    rootFS,
				SetUider:  setUider,
				Daemon:    daemon,
				Signaller: signaller,
				Waiter:    waiter,
			}
		})

		AfterEach(func() {
			Expect(os.Chdir(workingDirectory)).To(Succeed())
		})

		It("waits for host", func() {
			Expect(cz.Run()).To(Succeed())
			Expect(waiter.WaitCallCount()).To(Equal(1))
		})

		Context("when the waiter fails", func() {
			BeforeEach(func() {
				waiter.WaitReturns(errors.New("Foo"))
			})

			It("returns an error", func() {
				Expect(cz.Run()).To(MatchError("containerizer: Failed to wait for host: Foo"))
			})

			It("does not initialize the daemon", func() {
				cz.Run()
				Expect(daemon.InitCallCount()).To(Equal(0))
			})
		})

		It("initializes the daemon", func() {
			err := cz.Run()
			Expect(err).ToNot(HaveOccurred())
			Expect(daemon.InitCallCount()).To(Equal(1))
		})

		Context("when the daemon initialization fails", func() {
			BeforeEach(func() {
				daemon.InitReturns(errors.New("Booo"))
			})

			It("returns an error", func() {
				Expect(cz.Run()).To(MatchError("containerizer: Failed to initialize daemon: Booo"))
			})

			It("does not enter rootfs", func() {
				cz.Run()
				Expect(rootFS.EnterCallCount()).To(Equal(0))
			})
		})

		It("enters the rootfs", func() {
			Expect(cz.Run()).To(Succeed())
			Expect(rootFS.EnterCallCount()).To(Equal(1))
		})

		Context("when enter rootfs fails", func() {
			BeforeEach(func() {
				rootFS.EnterReturns(errors.New("Opps"))
			})

			It("returns an error", func() {
				Expect(cz.Run()).To(MatchError("containerizer: Failed to enter root fs: Opps"))
			})

			It("does not set uid", func() {
				cz.Run()
				Expect(setUider.SetUidCallCount()).To(Equal(0))
			})
		})

		PIt("setus uid", func() {
			Expect(cz.Run()).To(Succeed())
			Expect(setUider.SetUidCallCount()).To(Equal(1))
		})

		PContext("when set uid fails", func() {
			BeforeEach(func() {
				setUider.SetUidReturns(errors.New("Opps"))
			})

			It("returns an error", func() {
				Expect(cz.Run()).To(MatchError("containerizer: Failed to set uid: Opps"))
			})

			It("does not run the daemon", func() {
				cz.Run()
				Expect(daemon.RunCallCount()).To(Equal(0))
			})
		})

		It("sends signal to host", func() {
			Expect(cz.Run()).To(Succeed())
			Expect(signaller.SignalSuccessCallCount()).To(Equal(1))
		})

		Context("when the signaller fails", func() {
			BeforeEach(func() {
				signaller.SignalSuccessReturns(errors.New("Dooo"))
			})

			It("returns an error", func() {
				Expect(cz.Run()).To(MatchError("containerizer: Failed to signal host: Dooo"))
			})

			It("does not run the daemon", func() {
				cz.Run()
				Expect(daemon.RunCallCount()).To(Equal(0))
			})
		})

		It("runs the daemon", func() {
			err := cz.Run()
			Expect(err).ToNot(HaveOccurred())
			Expect(daemon.RunCallCount()).To(Equal(1))
		})

		Context("when daemon fails to run", func() {
			It("return an error", func() {
				daemon.RunReturns(errors.New("Bump"))
				err := cz.Run()
				Expect(err).To(MatchError("containerizer: Failed to run daemon: Bump"))
			})
		})
	})
})
