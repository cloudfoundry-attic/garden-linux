package in_container_test

import (
	"log"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

var inContainer = os.Getenv("GARDEN_IN_CONTAINER_TEST_SUITE")

func TestInContainer(t *testing.T) {
	if inContainer == "" {
		log.Println("Skipping in-container tests. These are run separately inside a container.")
		return
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "InContainer Networking Suite")
}
