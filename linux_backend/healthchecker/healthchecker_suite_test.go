package healthchecker_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestHealthchecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Healthchecker Suite")
}
