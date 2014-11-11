// +build linux
package net_fence_test

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/milosgajdos83/tenus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var netFenceBin string
var containerInitBin string

var _ = Describe("Configure", func() {

	BeforeEach(func() {
		netFencePath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/fences/mains/net-fence", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		netFenceBin = string(netFencePath)

		containerInitPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/fences/net_fence/container-init", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		containerInitBin = string(containerInitPath)
	})

	It("configures a network interface in the global network namespace", func() {
		_, err := tenus.NewVethPairWithOptions("testHostIfcName", tenus.VethOptions{
			PeerName:   "testPeerIfcName",
			TxQueueLen: 1,
		})
		Ω(err).ShouldNot(HaveOccurred())

		ctr, err := createContainer(syscall.CLONE_NEWNET, netFenceBin,
			"-containerIfcName=testPeerIfcName",
			"-containerIP=10.2.3.1",
			"-gatewayIP=10.2.3.2",
			"-subnet=10.2.3.0/30",
		)
		Ω(err).ShouldNot(HaveOccurred())

		// ctr.pid holds the pid of the container's init process
		pid := ctr.cmd.Process.Pid

		// Move the container's ethernet interface into the network namespace.
		moveInterfaceToNamespace("testPeerIfcName", pid)

		ctr.proceed()

		Ω(ctr.terminate()).ShouldNot(HaveOccurred())
	})
})

func moveInterfaceToNamespace(ifc string, pid int) {
	cmd := exec.Command("ip", "link", "set", ifc, "netns", fmt.Sprintf("%d", pid))
	err := cmd.Run()
	Ω(err).ShouldNot(HaveOccurred())
}

type container struct {
	rendezvousChan chan string
	outputChan     chan interface{}
	cmd            *exec.Cmd
	fd             net.Conn
}

// Creates a collection of namespaces defined by cloneFlags and starts an init process.
// When the init process has reached a rendezvous point, returns.
func createContainer(cloneFlags int, executable string, args ...string) (*container, error) {
	err := checkRoot()
	if err != nil {
		return nil, err
	}

	initArgs := make([]string, 0, len(args)+1)
	initArgs = append(initArgs, executable)
	initArgs = append(initArgs, args...)

	cmd := exec.Command(containerInitBin, initArgs...)

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Cloneflags = uintptr(cloneFlags)
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	container := &container{
		rendezvousChan: make(chan string),
		outputChan:     make(chan interface{}),
	}

	go func() {
		defer func() {
			container.outputChan <- nil
		}()
		data := make([]byte, 1024)
		for {
			_, err := stdOut.Read(data)
			if err != nil {
				if err != io.EOF {
					fmt.Printf("Error reading standard output pipe: %s\n", err)
				}
				return
			}
		}
	}()
	go func() {
		defer func() {
			container.outputChan <- nil
		}()
		data := make([]byte, 1024)
		for {
			_, err := stdErr.Read(data)
			if err != nil {
				if err != io.EOF {
					fmt.Printf("Error reading standard error pipe: %s\n", err)
				}
				return
			}
		}
	}()

	go listenForClient(container)

	container.cmd = cmd
	err = cmd.Start()
	if err != nil {
		log.Fatal("Start failed:", err)
	}

	// wait for child to reach rendezvous point.
	err = container.rendezvous()
	if err != nil {
		log.Fatal("rendezvous failed:", err)
	}

	return container, nil
}

func listenForClient(ctr *container) {
	l, err := net.Listen("unix", "/tmp/test-rendezvous.sock")
	if err != nil {
		log.Fatal("listen error:", err)
	}
	ctr.fd, err = l.Accept()
	if err != nil {
		log.Fatal("accept error:", err)
	}
	lineReader := bufio.NewReader(ctr.fd)
	str, err := lineReader.ReadString('\n')
	if err != nil {
		log.Fatal("ReadString error:", err)
	}
	ctr.rendezvousChan <- str
}

func (c *container) rendezvous() error {
	str := <-c.rendezvousChan
	if str != "rendezvous\n" {
		log.Fatal("unexpected rendezvous string from client")
	}
	return nil
}

// Allows the container to proceed from the rendezbous point to run the executable to completion.
// Pre-condition: the container must be at the rendezvous point.
func (c *container) proceed() error {
	// let the child continue
	c.fd.Write([]byte("rendezvous\n"))
	return nil
}

func (c *container) terminate() error {
	<-c.outputChan
	<-c.outputChan
	return c.cmd.Wait()
}

func checkRoot() error {
	if uid := os.Getuid(); uid != 0 {
		return fmt.Errorf("createContainer must be run as root. Getuid returned %d", uid)
	}
	return nil
}
