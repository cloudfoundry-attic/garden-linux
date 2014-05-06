package lifecycle_test

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	Runner "github.com/cloudfoundry-incubator/warden-linux/integration/runner"
)

var runner *Runner.Runner
var client warden.Client

func TestLifecycle(t *testing.T) {
	binPath := "../../linux_backend/bin"
	rootFSPath := os.Getenv("WARDEN_TEST_ROOTFS")

	if rootFSPath == "" {
		log.Println("WARDEN_TEST_ROOTFS undefined; skipping")
		return
	}

	var tmpdir string

	BeforeSuite(func() {
		var err error

		tmpdir, err = ioutil.TempDir("", "warden-socket")
		Ω(err).ShouldNot(HaveOccurred())

		wardenPath, err := gexec.Build("github.com/cloudfoundry-incubator/warden-linux", "-race")
		Ω(err).ShouldNot(HaveOccurred())

		runner, err = Runner.New(wardenPath, binPath, rootFSPath, "unix", filepath.Join(tmpdir, "warden.sock"))
		Ω(err).ShouldNot(HaveOccurred())

		err = runner.Start()
		Ω(err).ShouldNot(HaveOccurred())

		client = runner.NewClient()
	})

	AfterSuite(func() {
		err := runner.Stop()
		Ω(err).ShouldNot(HaveOccurred())

		err = runner.TearDown()
		Ω(err).ShouldNot(HaveOccurred())

		err = os.RemoveAll(tmpdir)
		Ω(err).ShouldNot(HaveOccurred())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}
