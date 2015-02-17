package main

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	linkpkg "github.com/cloudfoundry-incubator/garden-linux/old/iodaemon/link"
	"github.com/kr/pty"
)

func link(socketPath string) int {
	l, err := linkpkg.Create(socketPath, os.Stdout, os.Stderr)
	if err != nil {
        logErr(err)
        return 2
	}

	resized := make(chan os.Signal, 10)

	go func() {
		for {
			<-resized

			rows, cols, err := pty.Getsize(os.Stdin)
			if err == nil {
				l.SetWindowSize(cols, rows)
			}
		}
	}()

	signal.Notify(resized, syscall.SIGWINCH)

	go io.Copy(l, os.Stdin)

	status, err := l.Wait()
	if err != nil {
		logErr(err)
        return 3
	}

	return status
}

func logErr(err error) {
    println("fatal: " + err.Error())
}