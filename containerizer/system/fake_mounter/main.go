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
		Type:       system.MountType(os.Args[1]),
		TargetPath: os.Args[2],
		Flags:      flags,
		Data:       os.Args[4],
	}

	if err := mnt.Mount(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		panic("mount failed!")
	}

	cmd := exec.Command(os.Args[5], os.Args[6:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
