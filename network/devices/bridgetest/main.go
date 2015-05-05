package main

import (
	"fmt"
	"net"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/network/devices"
)

func main() {
	b := devices.Bridge{}

	for i := 0; i < 10; i++ {
		_, subnet, _ := net.ParseCIDR("2.3.4.5/30")
		if _, err := b.Create("testbridge"+os.Args[1], net.ParseIP("1.2.3.4"), subnet); err != nil {
			fmt.Println(os.Stderr, "create bridge: ", err)
			os.Exit(2)
		}

		if err := b.Destroy("testbridge" + os.Args[1]); err != nil {
			fmt.Println(os.Stderr, "destroy bridge: ", err)
			os.Exit(3)
		}
	}
}
