package bind_mount_test

import (
	"fmt"
	"log"
	"os"
	_ "os/exec"
	"syscall"
	"testing"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var binPath = "../../old/linux_backend/bin" // relative to test suite directory
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var gardenBin string

var gardenRunner *runner.Runner
var gardenProcess ifrit.Process

var gardenClient api.Client

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

func TestBindMount(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	SynchronizedBeforeSuite(func() []byte {
		gardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())
		return []byte(gardenPath)
	}, func(gardenPath []byte) {
		gardenBin = string(gardenPath)
		gardenClient = startGarden()
	})

	SynchronizedAfterSuite(func() {
		gardenProcess.Signal(syscall.SIGKILL)
		Eventually(gardenProcess.Wait(), 10).Should(Receive())
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "BindMount Suite")
}

func containerIP(ctr api.Container) string {
	info, err := ctr.Info()
	Ω(err).ShouldNot(HaveOccurred())
	return info.ContainerIP
}
