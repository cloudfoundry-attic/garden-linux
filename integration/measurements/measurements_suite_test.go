package measurements_test

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

	Runner "github.com/cloudfoundry-incubator/warden-linux/integration/runner"
)

var binPath = "../../linux_backend/bin"
var rootFSPath = os.Getenv("WARDEN_TEST_ROOTFS")
var graphPath = os.Getenv("WARDEN_TEST_GRAPHPATH")

var wardenBin string

var wardenRunner *Runner.Runner
var wardenProcess ifrit.Process

var client warden.Client

func startWarden(argv ...string) warden.Client {
	wardenAddr := fmt.Sprintf("/tmp/warden_%d.sock", GinkgoParallelNode())

	wardenRunner = Runner.New("unix", wardenAddr, wardenBin, binPath, rootFSPath, graphPath, argv...)

	wardenProcess = ifrit.Envoke(wardenRunner)

	return wardenRunner.NewClient()
}

func restartWarden(argv ...string) {
	wardenProcess.Signal(syscall.SIGINT)
	Eventually(wardenRunner.TryDial, 10).Should(HaveOccurred())

	startWarden(argv...)
}

func TestLifecycle(t *testing.T) {
	if rootFSPath == "" {
		log.Println("WARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	BeforeSuite(func() {
		var err error

		wardenBin, err = gexec.Build("github.com/cloudfoundry-incubator/warden-linux", "-race")
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		wardenProcess.Signal(syscall.SIGKILL)
		Eventually(wardenProcess.Wait(), 5).Should(Receive())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Measurements Suite")
}
