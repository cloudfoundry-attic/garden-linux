package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "ERROR: Root directory not provided\n")
		os.Exit(1)
	}

	rootFS := &system.RootFS{
		Root: os.Args[1],
	}
	err := rootFS.Enter()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to enter root fs: %v\n", err)
		os.Exit(1)
	}

	files, err := ioutil.ReadDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to read directory: %v\n", err)
		os.Exit(1)
	}
	for _, file := range files {
		fmt.Printf("%v\t", file.Name())
	}
	fmt.Printf("\n")
}
