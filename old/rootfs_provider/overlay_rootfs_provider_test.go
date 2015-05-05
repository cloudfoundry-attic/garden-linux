package rootfs_provider_test

import (
	"errors"
	"os/exec"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
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
				rootfs, _, err := provider.ProvideRootFS(logger, "some-id", parseURL(""), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(rootfs).To(Equal("/some/overlays/path/some-id/rootfs"))

				Expect(fakeRunner).To(HaveExecutedSerially(
					fake_command_runner.CommandSpec{
						Path: "/some/bin/path/overlay.sh",
						Args: []string{"create", "/some/overlays/path/some-id", "/some/default/rootfs"},
					},
				))

			})
		})

		Context("with a path given", func() {
			It("executes overlay.sh create with the given rootfs", func() {
				rootfs, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"), false)
				Expect(err).ToNot(HaveOccurred())
				Expect(rootfs).To(Equal("/some/overlays/path/some-id/rootfs"))

				Expect(fakeRunner).To(HaveExecutedSerially(
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
					_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("/some/given/rootfs"), false)
					Expect(err).To(MatchError("overlay.sh: oh no!, this cake is not fresh"))
				})
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("executes overlay.sh cleanup for the id's path", func() {
			err := provider.CleanupRootFS(logger, "some-id")
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeRunner).To(HaveExecutedSerially(
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
				Expect(err).To(Equal(disaster))
			})
		})
	})
})
