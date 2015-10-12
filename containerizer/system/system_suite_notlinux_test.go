// +build !linux

package system_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSystem(t *testing.T) {
	It("contains low-level code that only works on linux", func() {
		Skip("Linux only suite")
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "System Suite")
}
