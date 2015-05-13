package container_daemon_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_ptyopener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_runner"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("Spawning", func() {
	Describe("Closing Pipes after Spawning", func() {
		var (
			runner            *fake_runner.FakeRunner
			ptyOpener         *fake_ptyopener.FakePTYOpener
			spawner           *container_daemon.Spawn
			originalOpenFiles int
		)

		openFileCount := func() int {
			procFd := fmt.Sprintf("/proc/%d/fd", os.Getpid())
			files, err := ioutil.ReadDir(procFd)
			Expect(err).ToNot(HaveOccurred())
			return len(files)
		}

		BeforeEach(func() {
			originalOpenFiles = openFileCount()

			runner = new(fake_runner.FakeRunner)
			ptyOpener = new(fake_ptyopener.FakePTYOpener)

			spawner = &container_daemon.Spawn{
				Runner: runner,
				PTY:    ptyOpener,
			}
		})

		Describe("with a tty", func() {
			var withTty = true

			Context("When starting fails", func() {
				BeforeEach(func() {
					runner.StartReturns(errors.New("failed"))
				})

				It("does not leak files", func() {
					for i := 0; i < 50; i++ {
						spawner.Spawn(exec.Command("foo"), withTty)
					}

					Eventually(openFileCount, "10s").Should(BeNumerically("<=", originalOpenFiles))
				})
			})
		})

		Describe("without a tty", func() {
			var withTty = false

			Context("When starting fails", func() {
				BeforeEach(func() {
					runner.StartReturns(errors.New("failed"))
				})

				It("does not leak files", func() {
					for i := 0; i < 50; i++ {
						spawner.Spawn(exec.Command("foo"), withTty)
					}

					Eventually(openFileCount, "10s").Should(BeNumerically("<=", originalOpenFiles))
				})
			})
		})
	})

	FDescribe("Integrate spawning with the process reaper", func() {
		var (
			reaper  *system.ProcessReaper
			spawner *container_daemon.Spawn
		)

		BeforeEach(func() {
			logger := lager.NewLogger("process_reaper_test_logger")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))
			reaper = system.StartReaper(logger)

			spawner = &container_daemon.Spawn{
				Runner: reaper,
			}
		})

		It("streams input to the process's stdin", func() {
			cmd := exec.Command("sh", "-c", "cat <&0")

			files, err := spawner.Spawn(cmd, false)
			Expect(err).ToNot(HaveOccurred())

			go copyAndClose(files[0], bytes.NewBufferString("hello\nworld"))

			println("About to issue Eventually", time.Now().Format(time.RFC3339))
			buffer := make([]byte, 512)
			n, err := files[1].Read(buffer)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(buffer[:n])).Should(Equal("hello\nworld"))

			println("About to Wait", time.Now().Format(time.RFC3339))
			n, err = files[3].Read(buffer)
			Expect(err).NotTo(HaveOccurred())
			Expect(buffer[0]).Should(Equal(byte(0)))
			println("Wait completed", time.Now().Format(time.RFC3339))
		})
	})
})

func copyAndClose(dst io.WriteCloser, src io.Reader) error {
	fmt.Fprintln(os.Stderr, "process.go: copyAndClose: about to start copying stdin", time.Now().Format(time.RFC3339))
	_, err := io.Copy(dst, src)
	fmt.Fprintln(os.Stderr, "process.go: copyAndClose: ended copying stdin", time.Now().Format(time.RFC3339))
	dst.Close() // Ignore error
	fmt.Fprintln(os.Stderr, "process.go: copyAndClose: stdin file descriptor closed", time.Now().Format(time.RFC3339))
	return err
}
