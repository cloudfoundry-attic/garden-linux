package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	libPath := flag.String("lib", "./lib", "Directory containing hooks")
	rootFsPath := flag.String("root", "", "Directory that will become root in the new mount namespace")
	runPath := flag.String("run", "./run", "Directory where server socket is placed")
	userNsFlag := flag.String("userns", "enabled", "If specified, use user namespacing")
	title := flag.String("title", "", "")
	flag.Parse()

	if *rootFsPath == "" {
		missing("--root")
	}

	binPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to obtain absolute path: %s", err)
		os.Exit(6)
	}

	socketPath := path.Join(*runPath, "wshd.sock")

	privileged := false
	if *userNsFlag == "" || *userNsFlag == "disabled" {
		privileged = true
	}

	containerReader, hostWriter, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to create pipe: %s", err)
		os.Exit(5)
	}

	hostReader, containerWriter, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to create pipe: %s", err)
		os.Exit(4)
	}

	sync := &containerizer.PipeSynchronizer{
		Reader: hostReader,
		Writer: hostWriter,
	}

	env, err := process.EnvFromFile(path.Join(*libPath, "../etc/config"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to read env from config file: %s", err)
		os.Exit(3)
	}

	uidMappingOffset, err := strconv.Atoi(env["root_uid"])
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to parse uid mapping offset from etc/config: %s", err)
		os.Exit(7)
	}

	listener, err := unix_socket.NewListenerFromPath(socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to create listener: %s", err)
		os.Exit(8)
	}

	socketFile, err := listener.File()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wshd: failed to obtain listener file: %s", err)
		os.Exit(9)
	}

	beforeCloneInitializer := &system.Initializer{
		Steps: []system.StepRunner{
		// &conainerizer.FuncStep{system.Mount{
		// 	Type:  system.Tmpfs,
		// 	Flags: syscall.MS_NODEV,
		// 	Path:  "/dev/shm",
		// }.Mount},
		},
	}

	cz := containerizer.Containerizer{
		Rlimits:                &container_daemon.RlimitsManager{},
		BeforeCloneInitializer: beforeCloneInitializer,
		InitBinPath:            path.Join(binPath, "initc"),
		InitArgs: []string{
			"--root", *rootFsPath,
			"--config", path.Join(*libPath, "../etc/config"),
			"--title", *title,
		},
		Execer: &system.NamespacingExecer{
			CommandRunner:    linux_command_runner.New(),
			ExtraFiles:       []*os.File{containerReader, containerWriter, socketFile},
			Privileged:       privileged,
			UidMappingOffset: uidMappingOffset,
		},
		Signaller: sync,
		Waiter:    sync,
		// Temporary until we merge the hook scripts functionality in Golang
		CommandRunner: linux_command_runner.New(),
		LibPath:       *libPath,
		RootfsPath:    *rootFsPath,
	}

	err = cz.Create()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create container: %s", err)
		os.Exit(2)
	}
}

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "%s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}
