package container_daemon

import (
	"fmt"
	"io/ioutil"
	"os"
)

type NoPidfile struct{}

func (p NoPidfile) Write(pid int) error {
	return nil
}

func (p NoPidfile) Remove() {
}

type Pidfile struct {
	Path string
}

func (p Pidfile) Write(pid int) error {
	return ioutil.WriteFile(p.Path, []byte(fmt.Sprintf("%d\n", pid)), 0700)
}

func (p Pidfile) Remove() {
	os.Remove(p.Path)
}
