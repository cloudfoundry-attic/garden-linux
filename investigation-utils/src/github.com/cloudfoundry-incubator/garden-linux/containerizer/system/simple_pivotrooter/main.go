// +build linux
package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	fmt.Fprintf(os.Stderr, "Boo!\n")
	msg := os.Args[2]

	fmt.Fprintf(os.Stderr, "%s: about to change to %s\n", msg, os.Args[1])
	if err := os.Chdir(os.Args[1]); err != nil {
		panic(fmt.Sprintf("system: failed to change directory into the bind mounted rootfs dir: %s", err))
	}

	fmt.Fprintf(os.Stderr, "%s: about to mkdir\n", msg)
	if err := os.MkdirAll("tmp/garden-host", 0700); err != nil {
		panic(fmt.Sprintf("system: mkdir: %s", err))
	}

	fmt.Fprintf(os.Stderr, "%s: about to pivot\n", msg)
	if err := syscall.PivotRoot(".", "tmp/garden-host"); err != nil {
		panic(fmt.Sprintf("system: failed to pivot root: %s", err))
	}
	fmt.Fprintf(os.Stderr, "%s: about to return\n", msg)

}
