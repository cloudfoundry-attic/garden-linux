package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
)

func main() {
	container_daemon.Detach(os.Args[1], os.Args[2])
	fmt.Println("detached")
	time.Sleep(120 * time.Second)
}
