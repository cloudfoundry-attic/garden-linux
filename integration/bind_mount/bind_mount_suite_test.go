package bind_mount_test

import (
	"fmt"
	"log"
	"os"
	_ "os/exec"
	"syscall"
	"testing"

	"github.com/cloudfoundry-incubator/garden"
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

var gardenClient garden.Client

func startGarden(argv ...string) garden.Client {
	gardenAddr := fmt.Sprintf("/tmp/garden_%d.sock", GinkgoParallelNode())

	{ // Check this test suite is in the correct directory
		b, err := os.Open(binPath)
		Expect(err).ToNot(HaveOccurred())
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
		var gardenPath string
		var err error

		useGshGshd := os.Getenv("USE_GSH_GSHD")
		if useGshGshd != "" {
			gardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon, USE_GSH_GSHD")
		} else {
			gardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		}

		Expect(err).ToNot(HaveOccurred())
		return []byte(gardenPath)
	}, func(gardenPath []byte) {
		gardenBin = string(gardenPath)
	})

	AfterEach(func() {
		ensureGardenRunning()
		gardenProcess.Signal(syscall.SIGTERM)
		Eventually(gardenProcess.Wait(), 10).Should(Receive())
	})

	SynchronizedAfterSuite(func() {
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "BindMount Suite")
}

func ensureGardenRunning() {
	if err := gardenClient.Ping(); err != nil {
		gardenClient = startGarden()
	}
	Expect(gardenClient.Ping()).ToNot(HaveOccurred())
}

func containerIP(ctr garden.Container) string {
	info, err := ctr.Info()
	Expect(err).ToNot(HaveOccurred())
	return info.ContainerIP
}
