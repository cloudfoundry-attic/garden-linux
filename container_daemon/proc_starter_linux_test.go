package container_daemon_test

import (
	"io/ioutil"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("proc_starter", func() {
	It("runs the process in the specified working directory", func() {
		testWorkDir, err := ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		cmd := exec.Command(procStarterBin, "ENCODEDRLIMITS=", "/bin/sh", "-c", "echo $PWD")
		cmd.Dir = testWorkDir
		op, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(op)).To(Equal(testWorkDir + "\n"))
	})
})
