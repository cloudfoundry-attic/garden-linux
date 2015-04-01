package linux_backend

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"

	"strconv"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type Config struct {
	Network json.RawMessage `json:"network"`
}

func RegisterHooks(hs hook.HookSet, runner Runner, config process.Env, container ContainerInitializer) {
	hs.Register(hook.PARENT_BEFORE_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-before-clone.sh")))
	})

	hs.Register(hook.PARENT_AFTER_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-after-clone.sh")))
	})

	hs.Register(hook.CHILD_AFTER_PIVOT, func() {
		_, ipNet, err := net.ParseCIDR(config["network_cidr"])
		must(err)

		mtu, err := strconv.ParseInt(config["container_iface_mtu"], 0, 64)
		must(err)

		logger := cf_lager.New("linux_backend: hook.CHILD_AFTER_PIVOT")
		c := network.NewConfigurer(logger)

		must(c.ConfigureContainer(config["network_container_iface"],
			net.ParseIP(config["network_container_ip"]),
			net.ParseIP(config["network_host_ip"]),
			ipNet,
			int(mtu),
		))

		must(container.SetHostname(config["id"]))
		must(container.MountProc())
		must(container.MountTmp())

		// Temporary until /etc/seed functionality removed
		if _, err := os.Stat("/etc/seed"); err == nil {
			must(exec.Command("/bin/sh", "-c", ". /etc/seed").Run())
		}
	})
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type Runner interface {
	Run(*exec.Cmd) error
}
