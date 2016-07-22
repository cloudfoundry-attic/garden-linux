package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/garden-linux/iodaemon"
	"code.cloudfoundry.org/lager"
)

const USAGE = `usage:

	iodaemon spawn [-timeout timeout] [-tty] <socket> <path> <args...>:
		spawn a subprocess, making its stdio and exit status available via
		the given socket
`

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

var windowColumns = flag.Int(
	"windowColumns",
	80,
	"initial window columns for the process's tty",
)

var windowRows = flag.Int(
	"windowRows",
	24,
	"initial window rows for the process's tty",
)

func main() {
	flag.Parse()

	args := flag.Args()

	switch args[0] {
	case "spawn":
		if len(args) < 3 {
			usage()
		}

		spawn(args)

	default:
		usage()
	}
}

func spawn(args []string) {
	logger := lager.NewLogger("iodaemon")
	sink := lager.NewReconfigurableSink(lager.NewWriterSink(os.Stderr, lager.DEBUG), lager.DEBUG)
	logger.RegisterSink(sink)

	wirer := &iodaemon.Wirer{WithTty: *tty, WindowColumns: *windowColumns, WindowRows: *windowRows}
	daemon := &iodaemon.Daemon{WithTty: *tty}

	if err := iodaemon.Spawn(logger, args[1], args[2:], *timeout, os.Stdout, wirer, daemon); err != nil {
		logger.Info("failed", lager.Data{"error": err.Error()})
		fmt.Fprintf(os.Stderr, "failed: %s", err)
		os.Exit(2)
	}

	os.Exit(0)
}

func usage() {
	println(USAGE)
	os.Exit(1)
}
