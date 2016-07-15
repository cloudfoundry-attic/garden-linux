package networking_test

import (
	"encoding/json"
	"net"
	"os"

	"code.cloudfoundry.org/garden-linux/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
	"time"
)

var externalIP net.IP
var client *runner.RunningGarden

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func stopGarden() {
	Expect(client.Stop()).To(Succeed())
}

func killGarden() {
	client.Kill()
}

func TestNetworking(t *testing.T) {
	var beforeSuite struct {
		ExampleDotCom net.IP
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error

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

		externalIP = beforeSuite.ExampleDotCom
	})

	BeforeEach(func() {
		if os.Getenv("GARDEN_TEST_ROOTFS") == "" {
			Skip("GARDEN_TEST_ROOTFS undefined")
		}
	})

	AfterEach(func() {
		err := client.DestroyAndStop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
	})

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Suite")
}
