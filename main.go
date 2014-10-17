package main

import (
	"flag"
	"github.com/cloudfoundry-incubator/garden-linux/net_fence"
	"github.com/cloudfoundry-incubator/garden-linux/old"
)

// garden-linux server process
func main() {

	net_fence.InitializeFlags(flag.CommandLine)
	flag.Parse()

	subnets, err := net_fence.Initialize()
	if err != nil {
		panic("failed to initialize net_fence")
	}

	old.Main(subnets)
}
