package container_daemon_test

import (
	"io/ioutil"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("proc_starter", func() {

	var procStarter string

	BeforeEach(func() {
		var err error
		procStarter, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/proc_starter")
		Expect(err).ToNot(HaveOccurred())
	})

	It("runs the process in the specified working directory", func() {
		testWorkDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		cmd := exec.Command(procStarter, "/bin/sh", "-c", "echo $PWD")
		cmd.Dir = testWorkDir
		op, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal(testWorkDir + "\n"))
	})
})
