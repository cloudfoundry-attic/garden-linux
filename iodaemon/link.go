package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/kr/pty"
)

func link(socketPath string) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fatal(err)
	}

	var b [2048]byte
	var oob [2048]byte

	n, oobn, _, _, err := conn.(*net.UnixConn).ReadMsgUnix(b[:], oob[:])
	if err != nil {
		log.Fatalln("failed to read unix msg:", err, n, oobn)
	}

	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		log.Fatalln("failed to parse socket control message:", err)
	}

	if len(scms) < 1 {
		log.Fatalln("no socket control messages sent")
	}

	scm := scms[0]

	fds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		log.Fatalln("failed to parse unix rights", err)
	}

	if len(fds) != 3 {
		log.Fatalln("invalid number of fds; need 3, got", len(fds))
	}

	stdout := os.NewFile(uintptr(fds[0]), "stdout")
	stderr := os.NewFile(uintptr(fds[1]), "stderr")
	status := os.NewFile(uintptr(fds[2]), "status")

	streaming := &sync.WaitGroup{}

	inputWriter := &inputWriter{gob.NewEncoder(conn)}

	resized := make(chan os.Signal, 10)

	go func() {
		for {
			<-resized

			rows, cols, err := pty.Getsize(os.Stdin)
			if err == nil {
				inputWriter.SetWindowSize(cols, rows)
			}
		}
	}()

	signal.Notify(resized, syscall.SIGWINCH)

	// do not add stdin to the waitgroup; it appears to cause things to hang.
	// doesn't make much sense anyway; if stdout/stderr closed we probably
	// can't write any more to stdin in the first place.
	go func() {
		io.Copy(inputWriter, os.Stdin)
		inputWriter.Close()
	}()

	streaming.Add(1)
	go func() {
		io.Copy(os.Stdout, stdout)
		stdout.Close()
		streaming.Done()
	}()

	streaming.Add(1)
	go func() {
		io.Copy(os.Stderr, stderr)
		stderr.Close()
		streaming.Done()
	}()

	streaming.Wait()

	var exitStatus int
	_, err = fmt.Fscanf(status, "%d\n", &exitStatus)
	if err != nil {
		log.Fatalln("failed to read status:", err)
	}

	os.Exit(exitStatus)
}
