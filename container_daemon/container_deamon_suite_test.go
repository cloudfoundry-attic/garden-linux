package container_daemon_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestContainerDeamon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContainerDeamon Suite")
}
