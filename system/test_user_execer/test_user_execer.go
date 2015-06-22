package main

import (
	"flag"

	"github.com/cloudfoundry-incubator/garden-linux/system"
)

func main() {
	uid := flag.Int("uid", -1, "uid")
	gid := flag.Int("gid", -1, "gid")
	flag.Parse()

	execer := system.UserExecer{}
	if err := execer.ExecAsUser(*uid, *gid, "bash", "-c", "id -u && id -g"); err != nil {
		panic(err)
	}
}
