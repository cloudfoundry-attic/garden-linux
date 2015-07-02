package lifecycle_test

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
)

var (
	shmTestBin        string
	capabilityTestBin string
	client            *runner.RunningGarden
)

func startGarden(argv ...string) *runner.RunningGarden {
	return runner.Start(argv...)
}

func restartGarden(argv ...string) {
	Expect(client.Ping()).To(Succeed(), "tried to restart garden while it was not running")
	Expect(client.Stop()).To(Succeed())
	client = startGarden(argv...)
}

func TestLifecycle(t *testing.T) {
	if os.Getenv("GARDEN_TEST_ROOTFS") == "" {
		log.Println("GARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	SetDefaultEventuallyTimeout(5 * time.Second) // CI is sometimes slow

	SynchronizedBeforeSuite(func() []byte {
		shmPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/lifecycle/shm_test")
		Expect(err).ToNot(HaveOccurred())

		os.Setenv("CGO_ENABLED", "1")
		capabilityPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capability", "-a", "-installsuffix", "static")
		Expect(err).ToNot(HaveOccurred())
		os.Unsetenv("CGO_ENABLED")

		os.Chmod(capabilityPath, 777)
		os.Chown(capabilityPath, 0, 0)

		capabilityDir := path.Dir(capabilityPath)
		os.Chmod(capabilityDir, 777)
		os.Chown(capabilityDir, 0, 0)

		data := fmt.Sprintf("%s|%s", shmPath, capabilityPath)
		return []byte(data)
	}, func(path []byte) {
		data := string(path)
		Expect(data).NotTo(BeEmpty())
		args := strings.Split(data, "|")
		shmTestBin = args[0]
		capabilityTestBin = args[1]
	})

	AfterEach(func() {
		err := client.DestroyAndStop()
		client.Cleanup()
		Expect(err).NotTo(HaveOccurred())
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
