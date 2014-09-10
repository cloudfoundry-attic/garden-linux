package lifecycle_test

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"testing"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	Runner "github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var binPath = "../../linux_backend/bin"
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var wardenBin string

var wardenRunner *Runner.Runner
var wardenProcess ifrit.Process

var client warden.Client

func startGarden(argv ...string) warden.Client {
	wardenAddr := fmt.Sprintf("/tmp/warden_%d.sock", GinkgoParallelNode())

	wardenRunner = Runner.New("unix", wardenAddr, wardenBin, binPath, rootFSPath, graphPath, argv...)

	wardenProcess = ifrit.Envoke(wardenRunner)

	return wardenRunner.NewClient()
}

func restartGarden(argv ...string) {
	wardenProcess.Signal(syscall.SIGINT)
	Eventually(wardenProcess.Wait(), 10).Should(Receive())

	startGarden(argv...)
}

func TestLifecycle(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	SynchronizedBeforeSuite(func() []byte {
		wardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		return []byte(wardenPath)
	}, func(wardenPath []byte) {
		wardenBin = string(wardenPath)
	})

	AfterEach(func() {
		wardenProcess.Signal(syscall.SIGKILL)
		Eventually(wardenProcess.Wait(), 10).Should(Receive())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}
