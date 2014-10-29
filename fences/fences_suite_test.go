package fences_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestFences(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fences Suite")
}
