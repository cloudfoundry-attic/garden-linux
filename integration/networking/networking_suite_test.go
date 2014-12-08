package networking_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"testing"
)

var binPath = "../../old/linux_backend/bin" // relative to test suite directory
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var gardenBin string
var netdogBin string

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

func TestNetworking(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var built struct {
		GardenPath string
		NetdogPath string
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		built.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())

		os.Setenv("CGO_ENABLED", "0")
		built.NetdogPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/networking/netdog", "-a")
		Ω(err).ShouldNot(HaveOccurred())

		b, err := json.Marshal(built)
		Ω(err).ShouldNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &built)
		Ω(err).ShouldNot(HaveOccurred())

		gardenBin = built.GardenPath
		netdogBin = built.NetdogPath
	})

	AfterEach(func() {
		gardenProcess.Signal(syscall.SIGKILL)
		Eventually(gardenProcess.Wait(), "10s").Should(Receive())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Suite")
}

// networking test utility functions
func containerIfName(container api.Container) string {
	return ifNamePrefix(container) + "-1"
}

func hostIfName(container api.Container) string {
	return ifNamePrefix(container) + "-0"
}

func ifNamePrefix(container api.Container) string {
	return "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle()
}
