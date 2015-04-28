package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func main() {
	flags, err := strconv.Atoi(os.Args[3])
	if err != nil {
		panic(err)
	}

	mnt := system.Mount{
		Type:  system.MountType(os.Args[1]),
		Path:  os.Args[2],
		Flags: flags,
	}

	if err := mnt.Mount(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
	}

	cmd := exec.Command(os.Args[4], os.Args[5:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
