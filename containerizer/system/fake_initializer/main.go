package main

import (
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_configurer"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	networkConfigurer := new(fake_configurer.FakeConfigurer)
	initializer := &system.Initializer{
		NetworkConfigurer: networkConfigurer,
		Root:              os.Args[1],
	}

	must(initializer.Init())

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
