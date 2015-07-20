// +build !linux

package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCapability(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Capability Suite")
}
