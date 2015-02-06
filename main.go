package main

import (
	"flag"
	"fmt"

	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/cloudfoundry-incubator/garden-linux/old"
)

// garden-linux server process
func main() {

	config, err := cnet.Init(flag.CommandLine)
	if err != nil {
		fmt.Printf("Error creating container network: %s", err)
		return
	}

	flag.Parse()

	builder, err := Main(config)
	if err != nil {
		fmt.Printf("Error creating container network: %s", err)
		return
	}

	old.Main(builder)
}

func Main(config *cnet.Config) (cnet.Builder, error) {
	builder, err := cnet.Main(config)
	if err != nil {
		return nil, err
	}
	return builder, nil
}
