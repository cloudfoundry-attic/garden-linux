package containerizer_test

import (
	"errors"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_container_execer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_initializer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_signaller"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_waiter"
	"github.com/cloudfoundry-incubator/garden-linux/hook"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Containerizer", func() {
	Describe("Create", func() {
		var cz *containerizer.Containerizer
		var containerExecer *fake_container_execer.FakeContainerExecer
		var signaller *fake_signaller.FakeSignaller
		var waiter *fake_waiter.FakeWaiter
		var hookCommandRunner *FakeCommandRunner
		var beforeCloneInitializer *fake_initializer.FakeInitializer

		BeforeEach(func() {
			containerExecer = &fake_container_execer.FakeContainerExecer{}
			signaller = &fake_signaller.FakeSignaller{}
			waiter = &fake_waiter.FakeWaiter{}
			hookCommandRunner = &FakeCommandRunner{}
			beforeCloneInitializer = &fake_initializer.FakeInitializer{}

			cz = &containerizer.Containerizer{
				BeforeCloneInitializer: beforeCloneInitializer,
				RootfsPath:             "some-rootfs",
				Execer:                 containerExecer,
				InitBinPath:            "initd",
				Signaller:              signaller,
				Waiter:                 waiter,
				// Temporary until we merge the hook scripts functionality in Golang
				CommandRunner: hookCommandRunner,
				LibPath:       "./lib",
			}
		})

		It("initializes resource limits", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(beforeCloneInitializer.InitCallCount()).To(Equal(1))
		})

		Context("when rlimits initialization fails", func() {
			BeforeEach(func() {
				beforeCloneInitializer.InitReturns(errors.New("Failed to apply hard rlimits"))
			})

			It("returns an error", func() {
				Expect(cz.Create()).To(MatchError("containerizer: before clone initializer: Failed to apply hard rlimits"))
			})

			It("does not call parent hooks", func() {
				Expect(cz.Create()).ToNot(Succeed())

				Expect(hookCommandRunner).ToNot(HaveExecutedSerially(
					CommandSpec{
						Path: "lib/hook",
						Args: []string{
							string(hook.PARENT_BEFORE_CLONE),
						},
					},
				))

				Expect(hookCommandRunner).ToNot(HaveExecutedSerially(
					CommandSpec{
						Path: "lib/hook",
						Args: []string{
							string(hook.PARENT_AFTER_CLONE),
						},
					},
				))
			})

			It("does not spawn the pivoter in the container", func() {
				Expect(cz.Create()).ToNot(Succeed())

				Expect(hookCommandRunner).ToNot(HaveExecutedSerially(
					CommandSpec{
						Path: "lib/pivotter",
						Args: []string{
							"-rootfs", "some-rootfs",
						},
					},
				))
			})
		})

		// Temporary until we merge the hook scripts functionality in Golang
		It("runs parent hooks", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(hookCommandRunner).To(HaveExecutedSerially(
				CommandSpec{
					Path: "lib/hook",
					Args: []string{
						string(hook.PARENT_BEFORE_CLONE),
					},
				},
				CommandSpec{
					Path: "lib/hook",
					Args: []string{
						string(hook.PARENT_AFTER_CLONE),
					},
				},
			))
		})

		It("spawns the pivoter process in to the container", func() {
			containerExecer.ExecReturns(42, nil)

			Expect(cz.Create()).To(Succeed())
			Expect(hookCommandRunner).To(HaveExecutedSerially(
				CommandSpec{
					Path: "lib/pivotter",
					Args: []string{
						"-rootfs", "some-rootfs",
					},
					Env: []string{
						"TARGET_NS_PID=42",
					},
				},
			))
		})

		It("runs the initd process in a container", func() {
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
				Expect(cz.Create()).To(MatchError("containerizer: create container: Oh my gawsh"))
			})

			// Temporary until we merge the hook scripts functionality in Golang
			It("does not run parent-after-clone hooks", func() {
				cz.Create()
				Expect(hookCommandRunner.ExecutedCommands()).To(HaveLen(1))
				Expect(hookCommandRunner).To(HaveExecutedSerially(
					CommandSpec{
						Path: "lib/hook",
						Args: []string{
							string(hook.PARENT_BEFORE_CLONE),
						},
					}))
			})

			It("does not signal the container", func() {
				cz.Create()
				Expect(signaller.SignalSuccessCallCount()).To(Equal(0))
			})
		})

		It("exports PID environment variable", func() {
			containerExecer.ExecReturns(12, nil)
			Expect(cz.Create()).To(Succeed())
			Expect(os.Getenv("PID")).To(Equal("12"))
		})

		It("sends signal to container", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(signaller.SignalSuccessCallCount()).To(Equal(1))
		})

		Context("when the signaller fails", func() {
			BeforeEach(func() {
				signaller.SignalSuccessReturns(errors.New("Dooo"))
			})

			It("returns an error", func() {
				Expect(cz.Create()).To(MatchError("containerizer: send success singnal to the container: Dooo"))
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
				Expect(cz.Create()).To(MatchError("containerizer: wait for container: Foo"))
			})
		})
	})

	Describe("Init", func() {
		var cz *containerizer.Containerizer
		var initializer *fake_initializer.FakeInitializer
		var signaller *fake_signaller.FakeSignaller
		var waiter *fake_waiter.FakeWaiter
		var workingDirectory string

		BeforeEach(func() {
			var err error

			workingDirectory, err = os.Getwd()
			Expect(err).ToNot(HaveOccurred())

			initializer = &fake_initializer.FakeInitializer{}
			signaller = &fake_signaller.FakeSignaller{}
			waiter = &fake_waiter.FakeWaiter{}

			cz = &containerizer.Containerizer{
				RootfsPath:           "",
				ContainerInitializer: initializer,
				Signaller:            signaller,
				Waiter:               waiter,
			}
		})

		AfterEach(func() {
			Expect(os.Chdir(workingDirectory)).To(Succeed())
		})

		It("initializes the container", func() {
			Expect(cz.Init()).To(Succeed())
			Expect(initializer.InitCallCount()).To(Equal(1))
		})

		Context("when container initialization fails", func() {
			BeforeEach(func() {
				initializer.InitReturns(errors.New("Bing"))
			})

			It("returns an error", func() {
				err := cz.Init()
				Expect(err).To(MatchError("containerizer: initializing the container: Bing"))
			})
		})
	})
})
