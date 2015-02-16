package rootfs_provider_test

import (
	"errors"
	"os/exec"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/rootfs_provider"
)

var _ = Describe("OverlayRootfsProvider", func() {
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

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/some/bin/path/overlay.sh",
						Args: []string{"create", "/some/overlays/path/some-id", "/some/given/rootfs"},
					},
					func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			Context("when the exit status is 222", func() {
				BeforeEach(func() {
					fakeRunner.WhenRunning(
						fake_command_runner.CommandSpec{
							Path: "/some/bin/path/overlay.sh",
							Args: []string{"create", "/some/overlays/path/some-id", "/some/given/rootfs"},
						},
						func(*exec.Cmd) error {
							return disaster
						},
					)
				})

				It("returns a helpful error message about nested aufs/overlayfs", func() {
					_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"))
					Ω(err).Should(MatchError("fstype does not support overlayfs/aufs"))
				})
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"))
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("executes overlay.sh cleanup for the id's path", func() {
			err := provider.CleanupRootFS(logger, "some-id")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeRunner).Should(HaveExecutedSerially(
				fake_command_runner.CommandSpec{
					Path: "/some/bin/path/overlay.sh",
					Args: []string{"cleanup", "/some/overlays/path/some-id"},
				},
			))

		})

		Context("when overlay.sh fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRunner.WhenRunning(
					fake_command_runner.CommandSpec{
						Path: "/some/bin/path/overlay.sh",
						Args: []string{"cleanup", "/some/overlays/path/some-id"},
					},
					func(*exec.Cmd) error {
						return disaster
					},
				)
			})

			It("returns the error", func() {
				err := provider.CleanupRootFS(logger, "some-id")
				Ω(err).Should(Equal(disaster))
			})
		})
	})
})
