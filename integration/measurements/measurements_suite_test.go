package measurements_test

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudfoundry-incubator/gordon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	Runner "github.com/cloudfoundry-incubator/warden-linux/integration/runner"
)

var runner *Runner.Runner
var client gordon.Client

func TestMeasurements(t *testing.T) {
	binPath := "../../linux_backend/bin"
	rootFSPath := os.Getenv("WARDEN_TEST_ROOTFS")

	if rootFSPath == "" {
		log.Println("WARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var err error

	tmpdir, err := ioutil.TempDir("", "warden-socket")
	if err != nil {
		log.Fatalln("failed to make dir for socker:", err)
	}

	wardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/warden-linux", "-race")
	if err != nil {
		log.Fatalln("failed to compile warden-linux:", err)
	}

	runner, err = Runner.New(wardenPath, binPath, rootFSPath, "unix", filepath.Join(tmpdir, "warden.sock"))
	if err != nil {
		log.Fatalln("failed to create runner:", err)
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Measurements Suite")

	err = runner.Stop()
	if err != nil {
		log.Fatalln("warden failed to stop:", err)
	}

	err = runner.TearDown()
	if err != nil {
		log.Fatalln("failed to tear down server:", err)
	}

	err = os.RemoveAll(tmpdir)
	if err != nil {
		log.Fatalln("failed to clean up socket dir:", err)
	}
}

var didRunGarden bool

var _ = BeforeEach(func() {
	if didRunGarden {
		return
	}
	didRunGarden = true
	err := runner.Start()
	if err != nil {
		log.Fatalln("warden failed to start:", err)
	}

	client = runner.NewClient()
})
