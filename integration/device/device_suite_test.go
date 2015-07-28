package device_test

import (
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
	BeforeEach(func() {
		if fuseRootFSPath == "" {
			Skip("GARDEN_FUSE_TEST_ROOTFS undefined")
		}
	})

	AfterEach(func() {
		err := client.DestroyAndStop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	RegisterFailHandler(Fail)
	RunSpecs(t, "Device Suite")
}
