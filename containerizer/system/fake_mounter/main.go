package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func main() {
	mnt := system.Mount{
		Type: system.MountType(os.Args[1]),
		Path: os.Args[2],
	}

	if err := mnt.Mount(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
	}

	cmd := exec.Command(os.Args[3], os.Args[4:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
