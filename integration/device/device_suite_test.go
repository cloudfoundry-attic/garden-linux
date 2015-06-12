package device_test

import (
	"log"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var fuseRootFSPath = os.Getenv("GARDEN_FUSE_TEST_ROOTFS")
var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func TestDevice(t *testing.T) {
	if fuseRootFSPath == "" {
		log.Println("GARDEN_FUSE_TEST_ROOTFS undefined; skipping")
		return
	}

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	AfterEach(func() {
		err := client.Stop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Device Suite")
}
