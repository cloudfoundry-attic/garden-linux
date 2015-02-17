package main

import (
	"time"

	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/old/iodaemon/link2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Iodaemon", func() {
	var (
		socketPath string
		tmpdir     string
	)
	BeforeEach(func() {
		var err error
		tmpdir, err = ioutil.TempDir("", "socket-dir")
		Ω(err).ShouldNot(HaveOccurred())

		socketPath = filepath.Join(tmpdir, "iodaemon.sock")
	})

	AfterEach(func() {
		os.RemoveAll(tmpdir)
	})

	It("spawns", func() {
		args := []string{"echo", "hello"}
		go spawn(socketPath, args, time.Second, false, 0, 0, false)

		linkStdout := gbytes.NewBuffer()
		Eventually(func() error {
			_, err := link2.Create(socketPath, linkStdout, os.Stderr)
			return err
		}, time.Second).ShouldNot(HaveOccurred())
		//        Eventually(linkStdout).Should(gbytes.Say("hello\n"))
	})

})
