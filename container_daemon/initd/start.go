package initd

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"code.cloudfoundry.org/garden-linux/container_daemon"
	"code.cloudfoundry.org/garden-linux/container_daemon/unix_socket"
	"code.cloudfoundry.org/garden-linux/containerizer"
	"code.cloudfoundry.org/garden-linux/containerizer/system"
	"github.com/docker/docker/pkg/reexec"
	"code.cloudfoundry.org/lager"

	// for rexec.Register("proc_starter")
	_ "code.cloudfoundry.org/garden-linux/container_daemon/proc_starter"
)

func init() {
	reexec.Register("initd", start)
}

// initd listens on a socket, spawns requested processes and reaps their
// exit statuses.
func start() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "initd: panicked: %s\n", r)
			os.Exit(4)
		}
	}()

	flag.String("title", "", "")
	dropCaps := flag.Bool("dropCapabilities", false, "drop capabilities before running processes")
	flag.Parse()

	container_daemon.Detach("/dev/null", "/dev/null")
	logger := lager.NewLogger("initd")

	syncWriter := os.NewFile(uintptr(4), "/dev/sync_writer")
	syscall.RawSyscall(syscall.SYS_FCNTL, uintptr(4), syscall.F_SETFD, syscall.FD_CLOEXEC)
	defer syncWriter.Close()

	sync := &containerizer.PipeSynchronizer{
		Writer: syncWriter,
	}

	reaper := system.StartReaper(logger, syscall.Wait4)
	defer reaper.Stop()

	daemon := &container_daemon.ContainerDaemon{
		CmdPreparer: &container_daemon.ProcessSpecPreparer{
			Users:   container_daemon.LibContainerUser{},
			Rlimits: &container_daemon.RlimitsManager{},
			Reexec: container_daemon.CommandFunc(func(args ...string) *exec.Cmd {
				return &exec.Cmd{
					Path: "/proc/self/exe",
					Args: args,
				}
			}),
			AlwaysDropCapabilities: *dropCaps,
		},
		Spawner: &container_daemon.Spawn{
			Runner: reaper,
			PTY:    system.KrPty,
		},
		Signaller: &container_daemon.ProcessSignaller{
			Logger: logger,
		},
	}

	socketFile := os.NewFile(uintptr(5), "/dev/host.sock")
	listener, err := unix_socket.NewListenerFromFile(socketFile)
	if err != nil {
		fail(fmt.Sprintf("initd: failed to create listener: %s\n", err), 5)
	}

	socketFile.Close()

	if err := sync.SignalSuccess(); err != nil {
		fail(fmt.Sprintf("signal host: %s", err), 6)
	}

	if err := daemon.Run(listener); err != nil {
		fail(fmt.Sprintf("run daemon: %s", err), 7)
	}
}

func fail(err string, code int) {
	fmt.Fprintf(os.Stderr, "initd: %s\n", err)
	os.Exit(code)
}
