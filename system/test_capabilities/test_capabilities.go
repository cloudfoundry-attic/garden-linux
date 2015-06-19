package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/system"
)

func main() {
	caps := system.ProcessCapabilities{Pid: os.Getpid()}
	if err := caps.Limit(); err != nil {
		panic(err)
	}
	fmt.Println("banana")

	time.Sleep(time.Hour)
}
