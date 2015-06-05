package networking_test

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
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

var gardenRunner *runner.Runner
var gardenProcess ifrit.Process

var client garden.Client
var externalIP net.IP

func startGarden(argv ...string) garden.Client {
	gardenAddr := fmt.Sprintf("/tmp/garden_%d.sock", GinkgoParallelNode())

	{ // Check this test suite is in the correct directory
		b, err := os.Open(binPath)
		Expect(err).ToNot(HaveOccurred())
		b.Close()
	}

	gardenRunner = runner.New("unix", gardenAddr, gardenBin, binPath, rootFSPath, graphPath, os.Getenv("BTRFS_SUPPORTED") != "", argv...)

	gardenProcess = ifrit.Invoke(gardenRunner)

	return gardenRunner.NewClient()
}

func ensureGardenRunning() {
	if err := client.Ping(); err != nil {
		client = startGarden()
	}
	Expect(client.Ping()).ToNot(HaveOccurred())
}

func checkConnection(container garden.Container, ip string, port int) error {
	process, err := container.Run(garden.ProcessSpec{
		User: "vcap",
		Path: "sh",
		Args: []string{"-c", fmt.Sprintf("echo hello | nc -w1 %s %d", ip, port)},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	if err != nil {
		return err
	}

	exitCode, err := process.Wait()
	if err != nil {
		return err
	}

	if exitCode == 0 {
		return nil
	} else {
		return fmt.Errorf("Request failed. Process exited with code %d", exitCode)
	}
}

func checkInternet(container garden.Container) error {
	return checkConnection(container, externalIP.String(), 80)
}

func TestNetworking(t *testing.T) {
	if rootFSPath == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var beforeSuite struct {
		GardenPath    string
		ExampleDotCom net.IP
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.GardenPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux", "-a", "-race", "-tags", "daemon")
		Expect(err).ToNot(HaveOccurred())

		ips, err := net.LookupIP("www.example.com")
		Expect(err).ToNot(HaveOccurred())
		Expect(ips).ToNot(BeEmpty())
		beforeSuite.ExampleDotCom = ips[0]

		b, err := json.Marshal(beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		gardenBin = beforeSuite.GardenPath
		externalIP = beforeSuite.ExampleDotCom
	})

	AfterEach(func() {
		ensureGardenRunning()
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

func dumpIP() {
	cmd := exec.Command("ip", "a")
	op, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP status:\n", string(op))

	cmd = exec.Command("iptables", "--verbose", "--exact", "--numeric", "--list")
	op, err = cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP tables chains:\n", string(op))

	cmd = exec.Command("iptables", "--list-rules")
	op, err = cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("IP tables rules:\n", string(op))
}
