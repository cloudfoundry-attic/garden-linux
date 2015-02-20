package networking_test

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
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

var client garden.Client
var externalIP net.IP

func startGarden(argv ...string) garden.Client {
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
	Ω(client.Ping()).Should(Succeed(), "tried to restart garden while it was not running")
	gardenProcess.Signal(syscall.SIGTERM)
	Eventually(gardenProcess.Wait(), "10s").Should(Receive())

	startGarden(argv...)
}

func TestNetworking(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var beforeSuite struct {
		GardenPath    string
		NetdogPath    string
		ExampleDotCom net.IP
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())

		oldCgo := os.Getenv("CGO_ENABLED")

		os.Setenv("CGO_ENABLED", "0")
		beforeSuite.NetdogPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/networking/netdog", "-a", "-installsuffix", "cgo")
		Ω(err).ShouldNot(HaveOccurred())

		if oldCgo == "" {
			os.Unsetenv("CGO_ENABLED")
		} else {
			os.Setenv("CGO_ENABLED", oldCgo)
		}

		ips, err := net.LookupIP("www.example.com")
		Ω(err).ShouldNot(HaveOccurred())
		Ω(ips).ShouldNot(BeEmpty())
		beforeSuite.ExampleDotCom = ips[0]

		b, err := json.Marshal(beforeSuite)
		Ω(err).ShouldNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Ω(err).ShouldNot(HaveOccurred())

		gardenBin = beforeSuite.GardenPath
		netdogBin = beforeSuite.NetdogPath
		externalIP = beforeSuite.ExampleDotCom
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
	RunSpecs(t, "Networking Suite")
}

// networking test utility functions
func containerIfName(container garden.Container) string {
	return ifNamePrefix(container) + "-1"
}

func hostIfName(container garden.Container) string {
	return ifNamePrefix(container) + "-0"
}

func ifNamePrefix(container garden.Container) string {
	return "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle()
}
