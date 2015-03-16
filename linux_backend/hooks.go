package linux_backend

import (
	"encoding/json"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
)

type Config struct {
	Network json.RawMessage `json:"network"`
}

func RegisterHooks(hs hook.HookSet, runner Runner) {
	hs.Register(hook.PARENT_BEFORE_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-before-clone.sh")))
	})

	hs.Register(hook.PARENT_AFTER_CLONE, func() {
		must(runner.Run(exec.Command("./hook-parent-after-clone.sh")))
	})

	hs.Register(hook.CHILD_BEFORE_PIVOT, func() {
		must(runner.Run(exec.Command("./hook-child-before-pivot.sh")))
	})

	hs.Register(hook.CHILD_AFTER_PIVOT, func() {
		must(runner.Run(exec.Command("./hook-child-after-pivot.sh")))
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
