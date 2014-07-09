package main_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var iodaemon string
var tmpdir string
var socketPath string

var _ = BeforeSuite(func() {
	var err error

	iodaemon, err = gexec.Build("github.com/cloudfoundry-incubator/warden-linux/iodaemon", "-race")
	Ω(err).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(gexec.CleanupBuildArtifacts)

var _ = BeforeEach(func() {
	var err error

	tmpdir, err = ioutil.TempDir("", "socket-dir")
	Ω(err).ShouldNot(HaveOccurred())

	socketPath = filepath.Join(tmpdir, "iodaemon.sock")
})

var _ = AfterEach(func() {
	os.RemoveAll(tmpdir)
})

func TestIodaemon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Iodaemon Suite")
}
