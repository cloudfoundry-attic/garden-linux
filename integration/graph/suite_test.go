package graph_test

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/garden-linux/integration/runner"
)

var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func TestGraph(t *testing.T) {
	BeforeEach(func() {
		if rootFSPath == "" {
			Skip("GARDEN_TEST_ROOTFS undefined")
		}
	})

	AfterEach(func() {
		err := client.DestroyAndStop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	RegisterFailHandler(Fail)
	RunSpecs(t, "Graph Suite")
}
