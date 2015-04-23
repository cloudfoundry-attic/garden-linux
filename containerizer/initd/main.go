package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/unix_socket"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func missing(flagName string) {
	fmt.Fprintf(os.Stderr, "%s is required\n", flagName)
	flag.Usage()
	os.Exit(1)
}

type networkConfigurer struct {
	Config map[string]string
}

func (nc *networkConfigurer) Configure() error {
	_, ipNet, err := net.ParseCIDR(nc.Config["network_cidr"])
	if err != nil {
		return err
	}

	mtu, err := strconv.ParseInt(nc.Config["container_iface_mtu"], 0, 64)
	if err != nil {
		return err
	}

	logger, _ := cf_lager.New("hook")
	configurer := network.NewConfigurer(logger.Session("linux_backend: hook.CHILD_AFTER_PIVOT"))
	err = configurer.ConfigureContainer(&network.ContainerConfig{
		Hostname:      nc.Config["id"],
		ContainerIntf: nc.Config["network_container_iface"],
		ContainerIP:   net.ParseIP(nc.Config["network_container_ip"]),
		GatewayIP:     net.ParseIP(nc.Config["network_host_ip"]),
		Subnet:        ipNet,
		Mtu:           int(mtu),
	})
	if err != nil {
		return err
	}

	return nil
}

func main() {
	socketPath := flag.String("socket", "", "Path for the socket file")
	rootFsPath := flag.String("root", "", "Path for the root file system directory")
	configFilePath := flag.String("config", "./etc/config", "Path for the configuration file")
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

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

	env, _ := process.EnvFromFile(*configFilePath)
	initializer := &system.Initializer{
		Root: "/",
		NetworkConfigurer: &networkConfigurer{
			Config: env,
		},
	}

	containerizer := containerizer.Containerizer{
		RootFS: &system.RootFS{
			Root: *rootFsPath,
		},
		Daemon: &container_daemon.ContainerDaemon{
			Listener: &unix_socket.Listener{
				SocketPath: *socketPath,
			},
			Runner: linux_command_runner.New(),
		},
		Initializer: initializer,
		Waiter:      sync,
		Signaller:   sync,
	}

	err := containerizer.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run containerizer: %s\n", err)
		os.Exit(2)
	}
}
