package main

import (
	"time"

	"io/ioutil"
	"os"
	"path/filepath"

	"bytes"
	"io"

	linkpkg "github.com/cloudfoundry-incubator/garden-linux/iodaemon/link"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

type wc struct {
	*bytes.Buffer
}

func (b wc) Close() error {
	return nil
}

var _ = Describe("Iodaemon", func() {
	var (
		socketPath       string
		tmpdir           string
		terminate        chan int
		fakeOut          wc
		fakeErr          wc
		expectedExitCode int
	)

	BeforeEach(func() {
		var err error
		expectedExitCode = 0
		tmpdir, err = ioutil.TempDir("", "socket-dir")
		Expect(err).ToNot(HaveOccurred())

		socketPath = filepath.Join(tmpdir, "iodaemon.sock")

		terminate = make(chan int, 1)

		fakeOut = wc{
			bytes.NewBuffer([]byte{}),
		}
		fakeErr = wc{
			bytes.NewBuffer([]byte{}),
		}
	})

	AfterEach(func() {
		defer os.RemoveAll(tmpdir)

		By("waiting for iodeamon to terminate")
		Eventually(terminate, "2s").Should(Receive(Equal(expectedExitCode)))

		By("tidying up the socket file")
		if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
			Fail("socket file not cleaned up")
		}
	})

	Context("spawning a process", func() {
		spawnProcess := func(args ...string) {
			go spawn(socketPath, args, time.Second, false, 0, 0, false, terminate, fakeOut, fakeErr)
		}

		It("times out when no listeners connect", func() {
			expectedExitCode = 2
			spawnProcess("echo", "hello")
		})

		It("reports back stdout", func() {
			spawnProcess("echo", "hello")

			_, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())
			Eventually(linkStdout).Should(gbytes.Say("hello\n"))
		})

		It("supports re-linking to an iodaemon instance", func() {
			spawnProcess("bash")

			l, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())
			err = l.Writer.TerminateConnection()
			Expect(err).ToNot(HaveOccurred())

			m, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			m.Write([]byte("exit\n"))
		})

		It("reports back stderr", func() {
			spawnProcess("bash", "-c", "echo error 1>&2")

			_, _, linkStderr, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())
			Eventually(linkStderr).Should(gbytes.Say("error\n"))
		})

		It("sends stdin to child", func() {
			spawnProcess("env", "-i", "bash", "--noprofile", "--norc")

			l, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Write([]byte("echo hello\n"))
			Eventually(linkStdout).Should(gbytes.Say(".*hello.*"))

			l.Write([]byte("exit\n"))
		})

		It("exits when the child exits", func() {
			spawnProcess("bash")

			l, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Write([]byte("exit\n"))
		})

		It("closes stdin when the link is closed", func() {
			spawnProcess("bash")

			l, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Close() //bash will normally terminate when it receives EOF on stdin
		})

		Context("when there is an existing socket file", func() {
			BeforeEach(func() {
				file, err := os.Create(socketPath)
				Expect(err).ToNot(HaveOccurred())
				file.Close()
			})

			It("still creates the process", func() {
				spawnProcess("echo", "hello")

				_, linkStdout, _, err := createLink(socketPath)
				Expect(err).ToNot(HaveOccurred())
				Eventually(linkStdout).Should(gbytes.Say("hello\n"))
			})
		})
	})

	Context("spawning a tty", func() {
		spawnTty := func(args ...string) {
			go spawn(socketPath, args, time.Second, true, 200, 80, false, terminate, fakeOut, fakeErr)
		}

		It("reports back stdout", func() {
			spawnTty("echo", "hello")

			_, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())
			Eventually(linkStdout).Should(gbytes.Say("hello"))
		})

		It("reports back stderr to stdout", func() {
			spawnTty("bash", "-c", "echo error 1>&2")

			_, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())
			Eventually(linkStdout).Should(gbytes.Say("error"))
		})

		It("exits when the child exits", func() {
			spawnTty("bash")

			l, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Write([]byte("exit\n"))
		})

		It("closes stdin when the link is closed", func() {
			spawnTty("bash")

			l, _, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Close() //bash will normally terminate when it receives EOF on stdin
		})

		It("sends stdin to child", func() {
			spawnTty("env", "-i", "bash", "--noprofile", "--norc")

			l, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Write([]byte("echo hello\n"))
			Eventually(linkStdout).Should(gbytes.Say(".*hello.*"))

			l.Write([]byte("exit\n"))
		})

		It("correctly sets the window size", func() {
			spawnTty("env", "-i", "bash", "--noprofile", "--norc")

			l, linkStdout, _, err := createLink(socketPath)
			Expect(err).ToNot(HaveOccurred())

			l.Write([]byte("echo $COLUMNS $LINES\n"))
			Eventually(linkStdout).Should(gbytes.Say(".*\\s200 80\\s.*"))

			l.SetWindowSize(100, 40)

			l.Write([]byte("echo $COLUMNS $LINES\n"))
			Eventually(linkStdout).Should(gbytes.Say(".*\\s100 40\\s.*"))

			l.Write([]byte("exit\n"))
		})
	})

})

func createLink(socketPath string) (*linkpkg.Link, io.WriteCloser, io.WriteCloser, error) {
	linkStdout := gbytes.NewBuffer()
	linkStderr := gbytes.NewBuffer()
	var l *linkpkg.Link
	var err error
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		l, err = linkpkg.Create(socketPath, linkStdout, linkStderr)
		if err == nil {
			break
		}
	}
	return l, linkStdout, linkStderr, err
}
