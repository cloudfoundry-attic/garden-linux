package lifecycle_test

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"testing"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	Runner "github.com/cloudfoundry-incubator/garden-linux/old/integration/runner"
)

var binPath = "../../linux_backend/bin"
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var gardenBin string

var gardenRunner *Runner.Runner
var gardenProcess ifrit.Process

var client api.Client

func startGarden(argv ...string) api.Client {
	gardenAddr := fmt.Sprintf("/tmp/garden_%d.sock", GinkgoParallelNode())

	gardenRunner = Runner.New("unix", gardenAddr, gardenBin, binPath, rootFSPath, graphPath, argv...)

	gardenProcess = ifrit.Envoke(gardenRunner)

	return gardenRunner.NewClient()
}

func restartGarden(argv ...string) {
	gardenProcess.Signal(syscall.SIGINT)
	Eventually(gardenProcess.Wait(), 10).Should(Receive())

	startGarden(argv...)
}

func TestLifecycle(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	SynchronizedBeforeSuite(func() []byte {
		gardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		return []byte(gardenPath)
	}, func(gardenPath []byte) {
		gardenBin = string(gardenPath)
	})

	AfterEach(func() {
		gardenProcess.Signal(syscall.SIGKILL)
		Eventually(gardenProcess.Wait(), 10).Should(Receive())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}
