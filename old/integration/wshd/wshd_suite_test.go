// +build linux

package wshd_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestWshd(t *testing.T) {
	if os.Getenv("GARDEN_TEST_ROOTFS") != "" {
		RegisterFailHandler(Fail)
		RunSpecs(t, "wshd Suite")
	}
}
