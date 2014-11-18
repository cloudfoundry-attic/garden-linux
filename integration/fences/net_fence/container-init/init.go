package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
)

// The init process is the process root of a test container.
//
// Arguments:
// 0 - the init process path
// 1 - the target process path
// 2, ... - target process arguments
func main() {

	args := os.Args
	if len(args) < 2 {
		panic("Insufficient init process arguments")
	}

	rendezvous()

	cmd := exec.Command(args[1], args[2:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GARDEN_IN_CONTAINER_TEST_SUITE=true")
	output, err := cmd.CombinedOutput()
	fmt.Printf("\n%s\n", string(output))
	if err != nil {
		log.Fatalf("Target process failed to run: %s\n", err)
	}
}

func rendezvous() {

	c, err := net.Dial("unix", "/tmp/test-rendezvous.sock")
	if err != nil {
		log.Fatalf("Dial failed: %s", err)
	}
	defer c.Close()
	c.Write([]byte("rendezvous\n"))

	lineReader := bufio.NewReader(c)
	str, err := lineReader.ReadString('\n')
	if err != nil {
		log.Fatal("ReadString error:", err)
	}
	if str != "rendezvous\n" {
		log.Fatal("unexpected rendezvous string from server")
	}
}

// Use all CPUs for scheduling goroutines. The default in Go 1.3 is to use only one CPU.
func init() {
	cpus := runtime.NumCPU()
	runtime.GOMAXPROCS(cpus)
}
