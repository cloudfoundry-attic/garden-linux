// spawn [-t <timeout>] <socket directory> <args...>
// link <socket directory>

package main

import (
	"flag"
	"os"
	"time"
)

const USAGE = `usage:

	iomux spawn [-timeout timeout] [-tty] <socket> <path> <args...>:
		spawn a subprocess, making its stdio and exit status available via
		the given socket

	iomux link <socket>:
		attach to a process via the given socket
`

// TODO actually do this
var timeout = flag.Duration(
	"timeout",
	10*time.Second,
	"time duration to wait on an initial link before giving up",
)

var tty = flag.Bool(
	"tty",
	false,
	"spawn child process with a tty",
)

func main() {
	flag.Parse()

	args := flag.Args()

	switch args[0] {
	case "spawn":
		if len(args) < 3 {
			usage()
		}

		spawn(args[1], args[2], args[2:], *timeout, *tty)

	case "link":
		if len(args) < 2 {
			usage()
		}

		link(args[1])

	default:
		usage()
	}
}

func usage() {
	println(USAGE)
	os.Exit(1)
}
