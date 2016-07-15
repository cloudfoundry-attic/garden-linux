package lifecycle_test

import (
	"os"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/garden-linux/integration/runner"
)

var shmTestBin string

var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func restartGarden(argv ...string) {
	Expect(client.Ping()).To(Succeed(), "tried to restart garden while it was not running")
	Expect(client.Stop()).To(Succeed())
	client = startGarden(argv...)
}

func TestLifecycle(t *testing.T) {
	SynchronizedBeforeSuite(func() []byte {
		shmPath, err := gexec.Build("code.cloudfoundry.org/garden-linux/integration/lifecycle/shm_test")
		Expect(err).ToNot(HaveOccurred())
		return []byte(shmPath)
	}, func(path []byte) {
		Expect(string(path)).NotTo(BeEmpty())
		shmTestBin = string(path)
	})

	BeforeEach(func() {
		if os.Getenv("GARDEN_TEST_ROOTFS") == "" {
			Skip("GARDEN_TEST_ROOTFS undefined")
		}
	})

	AfterEach(func() {
		Expect(client.DestroyAndStop()).To(Succeed())
		Expect(client.Cleanup()).To(Succeed())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}

func containerIP(ctr garden.Container) string {
	info, err := ctr.Info()
	Expect(err).ToNot(HaveOccurred())
	return info.ContainerIP
}
