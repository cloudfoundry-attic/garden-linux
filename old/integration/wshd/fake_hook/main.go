package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/onsi/gomega/gbytes"
)

func main() {
	os.Chdir(path.Dir(os.Args[0]))
	cmd := exec.Command(fmt.Sprintf("./hook-%s.sh", os.Args[1]))
	cmd.Stdin = gbytes.NewBuffer() // avoid errors due to /dev/null not existing in fake container
	cmd.Stdout = gbytes.NewBuffer()
	cmd.Stderr = gbytes.NewBuffer()
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
