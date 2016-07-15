package main

import (
	"flag"
	"fmt"
	"os"

	"code.cloudfoundry.org/garden-linux/system"
)

func main() {
	uid := flag.Int("uid", -1, "uid")
	gid := flag.Int("gid", -1, "gid")
	workDir := flag.String("workDir", "", "working directory")
	flag.Parse()

	execer := system.UserExecer{}
	if err := execer.ExecAsUser(*uid, *gid, *workDir, "bash", "-c", "id -u && id -G"); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(2)
	}
}
