// makes and listens to udp connections because busybox doesn't have a netcat that supports udp
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
)

func Listen(port int) {
	l, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	fmt.Println("listening on port: ", port)
	if err != nil {
		log.Fatal(err)
	}

	for {
		buff := make([]byte, 1024)
		rlen, _, err := l.ReadFromUDP(buff)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Print(string(buff[:rlen]))
	}
}

func Send(dest string) {
	addr, err := net.ResolveUDPAddr("udp", dest)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()
	bytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := conn.Write(bytes); err != nil {
		log.Fatal(err)
	}
}

func main() {
	switch os.Args[1] {
	case "listen":
		Listen(mustParsePort(os.Args[2]))
	case "send":
		Send(os.Args[2])
	default:
		panic(fmt.Sprintf("not a valid action: %s", os.Args[1]))
	}
}

func mustParsePort(port string) int {
	i, err := strconv.Atoi(port)
	if err != nil {
		panic(fmt.Sprintf("not a valid port: %s", port))
	}

	return i
}
