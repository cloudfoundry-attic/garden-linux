package system_test

import (
	//. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"
)

var _ = Describe("Execer", func() {
	// var execer *containerizer.Execer
	// var commandRunner *fake_command_runner.FakeCommandRunner

	// BeforeEach(func() {
	// 	commandRunner = fake_command_runner.New()
	// 	process := &os.Process{
	// 		Pid: 12,
	// 	}
	// 	commandRunner.RunInjectsProcessToCmd(process)

	// 	execer = &containerizer.Execer{
	// 		CommandRunner: commandRunner,
	// 	}
	// })

	// It("executes the given command", func() {
	// 	_, err := execer.Exec("something", "smthg")
	// 	Expect(err).To(Succeed())

	// 	Expect(commandRunner).To(HaveExecutedSerially(
	// 		fake_command_runner.CommandSpec{
	// 			Path: "something",
	// 			Args: []string{
	// 				"smthg",
	// 			},
	// 		},
	// 	))
	// })

	// It("returns the correct PID", func() {
	// 	pid, err := execer.Exec("something", "smthg")
	// 	Expect(pid).To(Equal(12))
	// 	Expect(err).ToNot(HaveOccurred())
	// })

	// It("sets the correct flags", func() {
	// 	_, err := execer.Exec("something", "smthg")
	// 	Expect(err).ToNot(HaveOccurred())

	// 	cmd := commandRunner.ExecutedCommands()[0]
	// 	Expect(cmd.SysProcAttr).ToNot(BeNil())
	// 	Expect(cmd.SysProcAttr.Cloneflags).To(Equal(uintptr(syscall.CLONE_NEWUTS | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID)))
	// })
})
