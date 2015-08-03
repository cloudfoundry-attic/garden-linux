package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	fmt.Printf("pid = %d\n", syscall.Getpid())

	// http://stackoverflow.com/a/18106962
	sigc := make(chan os.Signal, 32)

	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGUSR2,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	for {
		s := <-sigc
		fmt.Printf("Received signal %#v\n", s)
	}
}
