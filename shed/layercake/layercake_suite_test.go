package layercake_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLayercake(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Layercake Suite")
}
