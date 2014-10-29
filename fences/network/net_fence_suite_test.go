package network

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNetFence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Fence suite")
}
