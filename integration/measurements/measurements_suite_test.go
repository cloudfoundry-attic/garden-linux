package measurements_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"testing"

	gardenClient "github.com/cloudfoundry-incubator/garden/client"
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

var client gardenClient.Client

func startGarden(argv ...string) gardenClient.Client {
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
	Eventually(gardenRunner.TryDial, 10).Should(HaveOccurred())

	startGarden(argv...)
}

func TestLifecycle(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var rsyslogd *os.Process

	SynchronizedBeforeSuite(func() []byte {
		rsyslogConf, err := os.Create("/etc/rsyslog.d/51-test.conf")
		Ω(err).ShouldNot(HaveOccurred())

		_, err = rsyslogConf.Write([]byte(`#
# Please work.
#
$RepeatedMsgReduction off
:programname, startswith, "gmeasure" /var/log/gmeasure
`))
		Ω(err).ShouldNot(HaveOccurred())

		err = rsyslogConf.Close()
		Ω(err).ShouldNot(HaveOccurred())

		rsyslogdCmd := exec.Command("rsyslogd")
		rsyslogdCmd.Stdout = os.Stdout
		rsyslogdCmd.Stderr = os.Stderr

		err = rsyslogdCmd.Start()
		Ω(err).ShouldNot(HaveOccurred())

		rsyslogd = rsyslogdCmd.Process

		gardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())
		return []byte(gardenPath)
	}, func(gardenPath []byte) {
		gardenBin = string(gardenPath)
	})

	BeforeEach(func() {
		// os.Remove("/var/log/gmeasure")
	})

	AfterEach(func() {
		gardenProcess.Signal(syscall.SIGQUIT)
		Eventually(gardenProcess.Wait(), 5).Should(Receive())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
		rsyslogd.Signal(os.Kill)
		rsyslogd.Wait()
		os.Remove("/etc/rsyslog.d/51-test.conf")
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Measurements Suite")
}
