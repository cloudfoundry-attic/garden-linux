package networking_test

import (
	"fmt"
	"strconv"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
)

func checkConnection(container garden.Container, ip string, port int) error {
	process, err := container.Run(garden.ProcessSpec{
		User: "vcap",
		Path: "sh",
		Args: []string{"-c", fmt.Sprintf("echo hello | nc -w1 %s %d", ip, port)},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	if err != nil {
		return err
	}

	exitCode, err := process.Wait()
	if err != nil {
		return err
	}

	if exitCode == 0 {
		return nil
	} else {
		return fmt.Errorf("Request failed. Process exited with code %d", exitCode)
	}
}

func checkInternet(container garden.Container) error {
	return checkConnection(container, externalIP.String(), 80)
}

// networking test utility functions
func containerIfName(container garden.Container) string {
	return ifNamePrefix(container) + "-1"
}

func hostIfName(container garden.Container) string {
	return ifNamePrefix(container) + "-0"
}

func ifNamePrefix(container garden.Container) string {
	return "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle()
}
