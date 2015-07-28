package bind_mount_test

import (
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
	RunSpecs(t, "BindMount Suite")
}
