package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/garden-linux/hook"
	"code.cloudfoundry.org/garden-linux/linux_backend"
	"code.cloudfoundry.org/garden-linux/logging"
	"code.cloudfoundry.org/garden-linux/network"
	"code.cloudfoundry.org/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func main() {
	cflager.AddFlags(flag.CommandLine)
	flag.Parse()
	logger, _ := cflager.New("hook")

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
	runner := &logging.Runner{linux_command_runner.New(), logger}
	configurer := network.NewConfigurer(logger.Session("linux_backend: hook.CHILD_AFTER_PIVOT"))
	linux_backend.RegisterHooks(hook.DefaultHookSet, runner, config, configurer)

	hook.Main(os.Args[1:])
}
