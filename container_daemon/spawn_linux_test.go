package container_daemon_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_ptyopener"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Closing Pipes after Spawning", func() {
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
