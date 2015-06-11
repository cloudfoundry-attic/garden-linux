package bind_mount_test

import (
	"log"
	"os"
	"testing"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func TestBindMount(t *testing.T) {
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
	RunSpecs(t, "BindMount Suite")
}
