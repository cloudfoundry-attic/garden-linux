package networking_test

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
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
var externalIP net.IP

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

	var beforeSuite struct {
		GardenPath    string
		NetdogPath    string
		ExampleDotCom net.IP
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Ω(err).ShouldNot(HaveOccurred())

		// FIXME: netdog cannot be statically linked with Go 1.4, so use a checked-in version built with Go 1.3.1 for now.
		// See https://groups.google.com/forum/#!msg/golang-nuts/S2WDcm47bhA/W243-l49WDsJ
		// os.Setenv("CGO_ENABLED", "0")
		// beforeSuite.NetdogPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/networking/netdog", "-a")
		// Ω(err).ShouldNot(HaveOccurred())
		beforeSuite.NetdogPath = "../../integration/networking/netdog/netdog"

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
func containerIfName(container api.Container) string {
	return ifNamePrefix(container) + "-1"
}

func hostIfName(container api.Container) string {
	return ifNamePrefix(container) + "-0"
}

func ifNamePrefix(container api.Container) string {
	return "w" + strconv.Itoa(GinkgoParallelNode()) + container.Handle()
}
