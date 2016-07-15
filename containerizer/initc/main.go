package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/garden-linux/containerizer"
	"code.cloudfoundry.org/garden-linux/containerizer/system"
	"code.cloudfoundry.org/garden-linux/network"
	"code.cloudfoundry.org/garden-linux/process"
	sys "code.cloudfoundry.org/garden-linux/system"
	"github.com/docker/docker/pkg/reexec"

	// for rexec.Register("initd")
	_ "code.cloudfoundry.org/garden-linux/container_daemon/initd"
)

func init() {
	runtime.LockOSThread()
}

// initc initializes a newly created container and then execs to become
// the init process
func main() {
	if reexec.Init() {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "initc: panicked: %s\n", r)
			os.Exit(4)
		}
	}()

	rootFsPath := flag.String("root", "", "Path for the root file system directory")
	configFilePath := flag.String("config", "./etc/config", "Path for the configuration file")
	title := flag.String("title", "", "")
	cflager.AddFlags(flag.CommandLine)
	flag.Parse()

	if *rootFsPath == "" {
		missing("--root")
	}

	syncReader := os.NewFile(uintptr(3), "/dev/a")
	defer syncReader.Close()
	syncWriter := os.NewFile(uintptr(4), "/dev/d")
	defer syncWriter.Close()

	sync := &containerizer.PipeSynchronizer{
		Reader: syncReader,
		Writer: syncWriter,
	}

	if err := sync.Wait(2 * time.Minute); err != nil {
		fail(fmt.Sprintf("initc: wait for host: %s", err), 8)
	}

	env, err := process.EnvFromFile(*configFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initc: failed to get env from config file: %s\n", err)
		os.Exit(3)
	}

	dropCapabilities := env["root_uid"] != "0"
	procMountFlags := syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC
	sysMountFlags := syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RDONLY

	if dropCapabilities {
		procMountFlags = procMountFlags | syscall.MS_RDONLY
	}

	initializer := &system.Initializer{
		Steps: []system.StepRunner{
			&containerizer.FuncStep{system.Mount{
				Type:       system.Tmpfs,
				Flags:      syscall.MS_NODEV,
				TargetPath: "/dev/shm",
			}.Mount},
			&containerizer.FuncStep{system.Mount{
				Type:       system.Proc,
				Flags:      procMountFlags,
				TargetPath: "/proc",
			}.Mount},
			&containerizer.FuncStep{system.Mount{
				Type:       system.Sys,
				Flags:      sysMountFlags,
				TargetPath: "/sys",
			}.Mount},
			&containerizer.FuncStep{system.Mount{
				Type:       system.Devpts,
				TargetPath: "/dev/pts",
				Data:       "newinstance,ptmxmode=0666",
			}.Mount},
			&containerizer.FuncStep{system.Unmount{
				Dir: "/tmp/garden-host",
			}.Unmount},
			&containerizer.FuncStep{func() error {
				return setupNetwork(env)
			}},
			&containerizer.CapabilitiesStep{
				Drop:         dropCapabilities,
				Capabilities: &sys.ProcessCapabilities{Pid: os.Getpid()},
			},
		},
	}

	containerizer := containerizer.Containerizer{
		RootfsPath:           *rootFsPath,
		ContainerInitializer: initializer,
		Waiter:               sync,
		Signaller:            sync,
	}

	if err := containerizer.Init(); err != nil {
		fail(fmt.Sprintf("failed to init containerizer: %s", err), 2)
	}

	syscall.RawSyscall(syscall.SYS_FCNTL, uintptr(4), syscall.F_SETFD, 0)
	syscall.RawSyscall(syscall.SYS_FCNTL, uintptr(5), syscall.F_SETFD, 0)

	if err := syscall.Exec("/proc/self/exe", []string{"initd", fmt.Sprintf("-dropCapabilities=%t", dropCapabilities), fmt.Sprintf("-title=\"%s\"", *title)}, os.Environ()); err != nil {
		fail(fmt.Sprintf("failed to reexec: %s", err), 3)
	}
}

func fail(err string, code int) {
	fmt.Fprintf(os.Stderr, "initc: %s\n", err)
	os.Exit(code)
}

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "initc: %s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

func setupNetwork(env process.Env) error {
	_, ipNet, err := net.ParseCIDR(env["network_cidr"])
	if err != nil {
		return fmt.Errorf("initc: failed to parse network CIDR: %s", err)
	}

	mtu, err := strconv.ParseInt(env["container_iface_mtu"], 0, 64)
	if err != nil {
		return fmt.Errorf("initc: failed to parse container interface MTU: %s", err)
	}

	logger, _ := cflager.New("hook")
	configurer := network.NewConfigurer(logger.Session("initc: hook.CHILD_AFTER_PIVOT"))
	err = configurer.ConfigureContainer(&network.ContainerConfig{
		Hostname:      env["id"],
		ContainerIntf: env["network_container_iface"],
		ContainerIP:   net.ParseIP(env["network_container_ip"]),
		GatewayIP:     net.ParseIP(env["network_host_ip"]),
		Subnet:        ipNet,
		Mtu:           int(mtu),
	})
	if err != nil {
		return fmt.Errorf("initc: failed to configure container network: %s", err)
	}

	return nil
}
