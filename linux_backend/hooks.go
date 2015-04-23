package linux_backend

import (
	"encoding/json"
	"net"
	"os/exec"
	"strconv"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type Config struct {
	Network json.RawMessage `json:"network"`
}

//go:generate counterfeiter . ContainerInitializer
type ContainerInitializer interface {
	MountProc() error
	MountTmp() error
}

func RegisterHooks(hs hook.HookSet, runner Runner, config process.Env, containerInitializer ContainerInitializer, configurer network.Configurer) {
	hs.Register(hook.PARENT_BEFORE_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-before-clone.sh")))
	})

	hs.Register(hook.PARENT_AFTER_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-after-clone.sh")))
		must(configureHostNetwork(config, configurer))
	})

	// hs.Register(hook.CHILD_AFTER_PIVOT, func() {
	// 	must(configureContainerNetwork(config, configurer))

	// 	must(containerInitializer.MountProc())
	// 	must(containerInitializer.MountTmp())

	// 	// Temporary until /etc/seed functionality removed
	// 	if _, err := os.Stat("/etc/seed"); err == nil {
	// 		must(exec.Command("/bin/sh", "-c", ". /etc/seed").Run())
	// 	}
	// })
}

func configureHostNetwork(config process.Env, configurer network.Configurer) error {
	_, ipNet, err := net.ParseCIDR(config["network_cidr"])
	if err != nil {
		return err
	}

	mtu, err := strconv.ParseInt(config["container_iface_mtu"], 0, 64)
	if err != nil {
		return err
	}

	// Temporary until PID is passed in from Go rewrite of wshd.
	containerPid, _ := pidFromFile("../run/wshd.pid")
	if err != nil {
		return err
	}

	err = configurer.ConfigureHost(&network.HostConfig{
		HostIntf:      config["network_host_iface"],
		BridgeName:    config["bridge_iface"],
		BridgeIP:      net.ParseIP(config["network_host_ip"]),
		ContainerIntf: config["network_container_iface"],
		ContainerPid:  containerPid,
		Subnet:        ipNet,
		Mtu:           int(mtu),
	})
	if err != nil {
		return err
	}

	return nil
}

func configureContainerNetwork(config process.Env, configurer network.Configurer) error {

	_, ipNet, err := net.ParseCIDR(config["network_cidr"])
	if err != nil {
		return err
	}

	mtu, err := strconv.ParseInt(config["container_iface_mtu"], 0, 64)
	if err != nil {
		return err
	}

	err = configurer.ConfigureContainer(&network.ContainerConfig{
		Hostname:      config["id"],
		ContainerIntf: config["network_container_iface"],
		ContainerIP:   net.ParseIP(config["network_container_ip"]),
		GatewayIP:     net.ParseIP(config["network_host_ip"]),
		Subnet:        ipNet,
		Mtu:           int(mtu),
	})
	if err != nil {
		return err
	}

	return nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type Runner interface {
	Run(*exec.Cmd) error
}
