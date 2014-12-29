package device_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"syscall"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	"github.com/cloudfoundry-incubator/garden/api"
)

var binPath = "../../old/linux_backend/bin" // relative to test suite directory
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var fuseRootFSPath = os.Getenv("GARDEN_FUSE_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var gardenBin string
var gardenRunner *runner.Runner
var gardenProcess ifrit.Process

var client api.Client

func startGarden(argv ...string) api.Client {
	gardenAddr := fmt.Sprintf("/tmp/garden_%d.sock", GinkgoParallelNode())

	{ // Check this test suite is in the correct directory
		b, err := os.Open(binPath)
		Ω(err).ShouldNot(HaveOccurred())
		b.Close()
	}

	gardenRunner = runner.New("unix", gardenAddr, gardenBin, binPath, rootFSPath, graphPath, argv...)

	gardenProcess = ifrit.Invoke(gardenRunner)

	return gardenRunner.NewClient()
}

func restartGarden(argv ...string) {
	gardenProcess.Signal(syscall.SIGINT)
	Eventually(gardenProcess.Wait(), "10s").Should(Receive())

	startGarden(argv...)
}

func TestDevice(t *testing.T) {
	if fuseRootFSPath == "" {
		log.Println("GARDEN_FUSE_TEST_ROOTFS undefined; skipping")
		return
	}

	var beforeSuite struct {
		GardenPath string
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())

		b, err := json.Marshal(beforeSuite)
		Ω(err).ShouldNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Ω(err).ShouldNot(HaveOccurred())

		gardenBin = beforeSuite.GardenPath
	})

	AfterEach(func() {
		gardenProcess.Signal(syscall.SIGQUIT)
		Eventually(gardenProcess.Wait(), "10s").Should(Receive())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Device Suite")
}
