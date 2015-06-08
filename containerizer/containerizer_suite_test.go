package containerizer_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestContainerizer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Containerizer Suite")
}
