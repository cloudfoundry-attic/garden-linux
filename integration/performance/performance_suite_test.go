package performance_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestPerformance(t *testing.T) {
	BeforeEach(func() {
		if os.Getenv("GARDEN_PERFORMANCE") == "" {
			Skip("GARDEN_PERFORMANCE undefined")
		}
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Suite")
}
