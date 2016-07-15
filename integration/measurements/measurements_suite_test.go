package measurements_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/garden-linux/integration/runner"
)

var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func TestLifecycle(t *testing.T) {
	BeforeEach(func() {
		if os.Getenv("GARDEN_TEST_ROOTFS") == "" {
			Skip("GARDEN_TEST_ROOTFS undefined")
		}
	})

	AfterEach(func() {
		err := client.DestroyAndStop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Measurements Suite")
}
