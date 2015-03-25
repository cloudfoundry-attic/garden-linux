package main

import (
	"fmt"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	"github.com/pivotal-golang/lager"
)

func main() {
	oldWd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cwd := path.Dir(os.Args[0])
	os.Chdir(cwd)
	defer os.Chdir(oldWd)

	config, err := process.EnvFromFile("../etc/config")
	if err != nil {
		panic(fmt.Sprintf("error reading config file in hook: %s", err))
	}
	runner := &logging.Runner{linux_command_runner.New(), lager.NewLogger("hook")}
	linux_backend.RegisterHooks(hook.DefaultHookSet, runner, config, linux_backend.NewContainerInitializer())

	hook.Main(os.Args[1:])
}
