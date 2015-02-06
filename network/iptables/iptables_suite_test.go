package iptables_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestIptables(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Iptables Suite")
}
