package rootfs_provider_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
)

var _ = FDescribe("OverlayRootfsProvider", func() {
	var (
		fakeRunner *fake_command_runner.FakeCommandRunner

		provider RootFSProvider

		logger *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeRunner = fake_command_runner.New()

		provider = NewOverlay("/some/bin/path", "/some/overlays/path", "/some/default/rootfs", fakeRunner)

		logger = lagertest.NewTestLogger("test")
	})

	Describe("ProvideRootFS", func() {
		Context("with no path given", func() {
			It("executes overlay.sh create with the default rootfs", func() {
				rootfs, _, err := provider.ProvideRootFS(logger, "some-id", parseURL(""))
				Ω(err).ShouldNot(HaveOccurred())
				Ω(rootfs).Should(Equal("/some/overlays/path/some-id/rootfs"))

				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/some/bin/path/overlay.sh",
						Args: []string{"create", "/some/overlays/path/some-id", "/some/default/rootfs"},
					},
				))

			})
		})

		Context("with a path given", func() {
			It("executes overlay.sh create with the given rootfs", func() {
				rootfs, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"))
				Ω(err).ShouldNot(HaveOccurred())
				Ω(rootfs).Should(Equal("/some/overlays/path/some-id/rootfs"))

				Ω(fakeRunner).Should(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/some/bin/path/overlay.sh",
						Args: []string{"create", "/some/overlays/path/some-id", "/some/given/rootfs"},
					},
				))
			})
		})

		Context("when overlay.sh fails", func() {
			disaster := errors.New("oh no!")

			Context("and stderr contains an error message", func() {
				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: "/some/bin/path/overlay.sh",
							Args: []string{"create", "/some/overlays/path/some-id", "/some/given/rootfs"},
						},
						func(cmd *exec.Cmd) error {
							cmd.Stderr.Write([]byte("this cake is not fresh\n"))
							return disaster
						},
					)
				})

				It("returns the error message from stderr", func() {
					_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"))
					Ω(err).Should(MatchError("overlay.sh: oh no!, this cake is not fresh"))
				})
			})
		})
	})

	Describe("CleanupRootFS", func() {

		Context("when the root fs been mounted", func() {
			BeforeEach(func() {
				provider = NewOverlay("/some/bin/path", "/tmp/some/overlays/path", "/some/default/rootfs", fakeRunner)
				err := os.MkdirAll("/tmp/some/overlays/path", os.ModePerm)
				Ω(err).ShouldNot(HaveOccurred())
				err = os.MkdirAll("/tmp/mount_me", os.ModePerm)
				Ω(err).ShouldNot(HaveOccurred())

				err = syscall.Mount("/tmp/mount_me", "/tmp/some/overlays/path", "overlayfs", 0, "-n")
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				os.RemoveAll("/tmp/some/overlays/path")
			})

			It("removes the container path", func() {
				err := provider.CleanupRootFS(logger, "some-id")
				Ω(err).ShouldNot(HaveOccurred())
				if _, err := os.Open("/tmp/some/overlays/path"); !os.IsNotExist(err) {
					Fail("did not remove the container path")
				}
			})

			PContext("when CleanupRootFS fails", func() {
				BeforeEach(func() {
					os.Mkdir("/tmp/some/overlays/path/locked_down", 0)
				})

				AfterEach(func() {
					os.Chmod("/tmp/some/overlays/path", os.ModePerm)
				})

				It("returns the error", func() {
					err := provider.CleanupRootFS(logger, "some-id")
					Ω(err).Should(HaveOccurred())
				})
			})
		})
	})
})
