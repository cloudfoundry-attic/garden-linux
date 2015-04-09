package linux_backend

import (
	"encoding/json"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

type Config struct {
	Network json.RawMessage `json:"network"`
}

//go:generate counterfeiter . ContainerInitializer
type ContainerInitializer interface {
	SetHostname(hostname string) error
	MountProc() error
	MountTmp() error
}

func RegisterHooks(hs hook.HookSet, runner Runner, config process.Env, container ContainerInitializer) {
	hs.Register(hook.PARENT_BEFORE_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-before-clone.sh")))
	})

	hs.Register(hook.PARENT_AFTER_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-after-clone.sh")))
	})

	hs.Register(hook.CHILD_AFTER_PIVOT, func() {
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
