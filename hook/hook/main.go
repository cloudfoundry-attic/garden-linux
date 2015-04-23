package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/hook"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func main() {
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()
	logger, _ := cf_lager.New("hook")

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
	linux_backend.RegisterHooks(hook.DefaultHookSet, runner, config, nil, configurer)

	hook.Main(os.Args[1:])
}
