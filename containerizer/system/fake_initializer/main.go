package main

import (
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	initializer := &system.Initializer{
		Config: map[string]string{
			"id": os.Args[1],
		},
	}

	must(initializer.Init())

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
