package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func main() {
	mountType := flag.String("type", "", "Mount type")
	sourcePath := flag.String("sourcePath", "", "Source path")
	targetPath := flag.String("targetPath", "", "Destination path")
	flags := flag.Int("flags", -1, "Mount options")
	data := flag.String("data", "", "Data options")

	flag.Parse()

	mnt := system.Mount{
		Type:       system.MountType(*mountType),
		SourcePath: *sourcePath,
		TargetPath: *targetPath,
		Flags:      *flags,
		Data:       *data,
	}

	if err := mnt.Mount(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		panic("mount failed!")
	}

	args := flag.Args()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
