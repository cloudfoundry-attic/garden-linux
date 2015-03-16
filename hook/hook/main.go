package main

import (
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	"github.com/pivotal-golang/lager"
)

func init() {
	runner := &logging.Runner{linux_command_runner.New(), lager.NewLogger("hook")}
	linux_backend.RegisterHooks(hook.DefaultHookSet, runner)
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	os.Chdir(path.Dir(os.Args[0]))
	defer os.Chdir(cwd)

	hook.Main(os.Args[1:])
}
