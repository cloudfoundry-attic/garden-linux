package main

import (
	"fmt"
	"os"
	"time"

	"flag"

	"github.com/cloudfoundry-incubator/garden-linux/system"
)

func main() {
	extendedWhitelist := flag.Bool("extendedWhitelist", false, "")
	flag.Parse()

	caps := system.ProcessCapabilities{Pid: os.Getpid()}
	if err := caps.Limit(*extendedWhitelist); err != nil {
		panic(err)
	}
	fmt.Println("banana")

	time.Sleep(time.Hour)
}
