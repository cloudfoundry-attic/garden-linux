package iptables_manager_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestIptablesManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IptablesManager Suite")
}
