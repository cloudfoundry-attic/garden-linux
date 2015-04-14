package main

import (
	"os"
	"path"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func main() {
	binPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}

	cz := containerizer.Containerizer{
		InitBinPath: path.Join(binPath, "initd"),
		Execer: system.Execer{
			CommandRunner: linux_command_runner.New(),
		},
	}

	cz.Create()
}
