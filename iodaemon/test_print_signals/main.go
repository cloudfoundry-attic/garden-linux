package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
)

func main() {
	fmt.Printf("pid = %d\n", syscall.Getpid())

	extraFd := os.NewFile(3, "extrafd")
	msg := &struct{ Signal string }{}
	json.NewDecoder(extraFd).Decode(&msg)

	fmt.Println("Received:", msg.Signal)
}
