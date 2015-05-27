package lifecycle_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"encoding/json"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var binPath = "../../old/linux_backend/bin" // relative to test suite directory
var rootFSPath = os.Getenv("GARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("GARDEN_TEST_GRAPHPATH")

var gardenBin, shmTestBin string

var gardenRunner *runner.Runner
var gardenProcess ifrit.Process

var client garden.Client

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

func restartGarden(argv ...string) {
	Expect(client.Ping()).To(Succeed(), "tried to restart garden while it was not running")
	gardenProcess.Signal(syscall.SIGTERM)
	Eventually(gardenProcess.Wait(), 30).Should(Receive())

	startGarden(argv...)
}

func ensureGardenRunning() {
	if err := client.Ping(); err != nil {
		client = startGarden()
	}
	Expect(client.Ping()).ToNot(HaveOccurred())
}

func TestLifecycle(t *testing.T) {
	var beforeSuite struct {
		GardenPath  string
		ShmTestPath string
	}

	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Expect(err).ToNot(HaveOccurred())

		beforeSuite.ShmTestPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/lifecycle/shm_test")
		Expect(err).ToNot(HaveOccurred())

		b, err := json.Marshal(beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		gardenBin = beforeSuite.GardenPath
		Expect(gardenBin).NotTo(BeEmpty())

		shmTestBin = beforeSuite.ShmTestPath
		Expect(shmTestBin).NotTo(BeEmpty())
	})

	AfterEach(func() {
		ensureGardenRunning()
		gardenProcess.Signal(syscall.SIGQUIT)
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

func containerIP(ctr garden.Container) string {
	info, err := ctr.Info()
	Expect(err).ToNot(HaveOccurred())
	return info.ContainerIP
}

func dumpIP() {
	cmd := exec.Command("ip", "a")
	op, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP status:\n", string(op))

	cmd = exec.Command("iptables", "--list")
	op, err = cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP tables chains:\n", string(op))

	cmd = exec.Command("iptables", "--list-rules")
	op, err = cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP tables rules:\n", string(op))
}
