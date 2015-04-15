package main

import (
	"flag"
	"os"
	"path"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

func main() {
	socketPath := flag.String("socket", "/tmp/wshd.sock", "Path for the socket file")

	// ******************** TODO: remove old flags *****************
	flag.String("lib", "", "")
	flag.String("userns", "", "")
	flag.String("root", "", "")
	flag.String("run", "", "")
	flag.String("title", "", "")
	// ******************** TODO: remove old flags *****************

	flag.Parse()

	binPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}

	cz := containerizer.Containerizer{
		InitBinPath: path.Join(binPath, "initd"),
		InitArgs:    []string{"--socket", *socketPath},
		Execer: system.Execer{
			CommandRunner: linux_command_runner.New(),
		},
	}

	cz.Create()
}
