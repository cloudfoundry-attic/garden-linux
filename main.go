package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/old"
)

// garden-linux server process
func main() {
	builders, err := fences.Main(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Printf("Error creating fence: %s", err)
		return
	}

	old.Main(builders)
}
