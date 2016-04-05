package container_repository_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestContainerRepository(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContainerRepository Suite")
}
