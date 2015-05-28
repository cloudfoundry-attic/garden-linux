package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "initd: panicked: %s\n", r)
			os.Exit(4)
		}
	}()

	socketPath := flag.String("socket", "", "Path for the socket file")
	rootFsPath := flag.String("root", "", "Path for the root file system directory")
	configFilePath := flag.String("config", "./etc/config", "Path for the configuration file")
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("init")

	if *socketPath == "" {
		missing("--socket")
	}
	if *rootFsPath == "" {
		missing("--root")
	}

	sync := &containerizer.PipeSynchronizer{
		Reader: os.NewFile(uintptr(3), "/dev/a"),
		Writer: os.NewFile(uintptr(4), "/dev/d"),
	}

	env, err := process.EnvFromFile(*configFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initd: failed to get env from config file: %s\n", err)
		os.Exit(3)
	}

	reaper := system.StartReaper(logger)
	defer reaper.Stop()

	initializer := &system.ContainerInitializer{
		Steps: []system.Initializer{
			&step{system.Mount{
				Type:  system.Tmpfs,
				Flags: syscall.MS_NODEV,
				Path:  "/dev/shm",
			}.Mount},
			&step{system.Mount{
				Type:  system.Proc,
				Flags: syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC,
				Path:  "/proc",
			}.Mount},
			&step{system.Unmount{
				Dir: "/tmp/garden-host",
			}.Unmount},
			&step{func() error {
				return setupNetwork(env)
			}},
			&container_daemon.ShellRunnerStep{
				Runner: reaper,
				Path:   "/etc/seed",
			},
		},
	}

	daemon := &container_daemon.ContainerDaemon{
		CmdPreparer: &container_daemon.ProcessSpecPreparer{
			Users:           system.LibContainerUser{},
			Rlimits:         &container_daemon.RlimitsManager{},
			ProcStarterPath: "/sbin/proc_starter",
		},
		Spawner: &container_daemon.Spawn{
			Runner: reaper,
			PTY:    system.KrPty,
		},
	}

	containerizer := containerizer.Containerizer{
		RootfsPath:  *rootFsPath,
		Initializer: initializer,
		Daemon:      daemon,
		Waiter:      sync,
		Signaller:   sync,
	}

	listener, err := unix_socket.NewListenerFromPath(*socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initd: failed to create listener: %s\n", err)
		os.Exit(5)
	}

	err = containerizer.Run(listener)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initd: failed to run containerizer: %s\n", err)
		os.Exit(2)
	}
}

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "initd: %s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

func setupNetwork(env process.Env) error {
	_, ipNet, err := net.ParseCIDR(env["network_cidr"])
	if err != nil {
		return fmt.Errorf("initd: failed to parse network CIDR: %s", err)
	}

	mtu, err := strconv.ParseInt(env["container_iface_mtu"], 0, 64)
	if err != nil {
		return fmt.Errorf("initd: failed to parse container interface MTU: %s", err)
	}

	logger, _ := cf_lager.New("hook")
	configurer := network.NewConfigurer(logger.Session("initd: hook.CHILD_AFTER_PIVOT"))
	err = configurer.ConfigureContainer(&network.ContainerConfig{
		Hostname:      env["id"],
		ContainerIntf: env["network_container_iface"],
		ContainerIP:   net.ParseIP(env["network_container_ip"]),
		GatewayIP:     net.ParseIP(env["network_host_ip"]),
		Subnet:        ipNet,
		Mtu:           int(mtu),
	})
	if err != nil {
		return fmt.Errorf("initd: failed to configure container network: %s", err)
	}

	return nil
}

type step struct {
	fn func() error
}

func (s *step) Init() error {
	return s.fn()
}
