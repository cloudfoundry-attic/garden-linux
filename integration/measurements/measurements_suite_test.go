package measurements_test

import (
	"log"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func TestLifecycle(t *testing.T) {
	if os.Getenv("GARDEN_TEST_ROOTFS") == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	AfterEach(func() {
		err := client.Stop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Measurements Suite")
}
