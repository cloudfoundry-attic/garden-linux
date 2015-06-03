package btrfs_cleanup_test

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/btrfs_cleanup"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/fake_graph_driver"

	"github.com/cloudfoundry/gunk/command_runner/fake_command_runner"
	. "github.com/cloudfoundry/gunk/command_runner/fake_command_runner/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BtrfsCleanup", func() {

	var (
		cleaner              *btrfs_cleanup.Cleaner
		runner               *fake_command_runner.FakeCommandRunner
		graphDriver          *fake_graph_driver.FakeGraphDriver
		listSubvolumesOutput string
		layerId              = "the-layer"
		btrfsMountPoint      = "/absolute/btrfs_mount"

		listSubVolumeErr error
		graphDriverErr   error

		removedDirectories []string
	)

	BeforeEach(func() {
		removedDirectories = []string{}

		runner = fake_command_runner.New()
		graphDriver = new(fake_graph_driver.FakeGraphDriver)
		cleaner = &btrfs_cleanup.Cleaner{
			Runner:          runner,
			GraphDriver:     graphDriver,
			BtrfsMountPoint: btrfsMountPoint,
			RemoveAll: func(dir string) error {
				removedDirectories = append(removedDirectories, dir)
				return nil
			},
		}

		runner.WhenRunning(fake_command_runner.CommandSpec{
			Path: "btrfs",
			Args: []string{"subvolume", "list", btrfsMountPoint},
		}, func(cmd *exec.Cmd) error {
			_, err := cmd.Stdout.Write([]byte(listSubvolumesOutput))
			Expect(err).NotTo(HaveOccurred())
			return listSubVolumeErr
		})

		graphDriver.GetStub = func(id, label string) (string, error) {
			return "/absolute/btrfs_mount/relative/path/to/" + id, graphDriverErr
		}
	})

	Context("when there are no subvolumes", func() {
		BeforeEach(func() {
			listSubvolumesOutput = "\n"
		})

		It("does not invoke subvolume delete", func() {
			Expect(cleaner.Clean(layerId)).To(Succeed())
			Expect(runner).NotTo(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "btrfs",
				Args: []string{"subvolume", "delete", "/path/to/" + layerId},
			}))
		})

		It("does not delete any directories", func() {
			Expect(cleaner.Clean(layerId)).To(Succeed())
			Expect(removedDirectories).To(BeEmpty())
		})
	})

	Context("when there is a subvolume for the layer, but it does not contain nested subvolumes", func() {
		BeforeEach(func() {
			listSubvolumesOutput = "ID 257 gen 9 top level 5 path relative/path/to/" + layerId + "\n"
		})

		It("does not invoke subvolume delete", func() {
			Expect(cleaner.Clean(layerId)).To(Succeed())
			Expect(runner).NotTo(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "btrfs",
				Args: []string{"subvolume", "delete", "/absolute/btrfs_mount/relative/path/to/" + layerId},
			}))
		})

		It("does not delete any directories", func() {
			Expect(cleaner.Clean(layerId)).To(Succeed())
			Expect(removedDirectories).To(BeEmpty())
		})
	})

	Context("when there is a subvolume for the layer, and it contains nested subvolumes", func() {
		subvolume1 := fmt.Sprintf("%s/relative/path/to/%s/subvolume1", btrfsMountPoint, layerId)
		subvolume2 := fmt.Sprintf("%s/relative/path/to/%s/subvolume2", btrfsMountPoint, layerId)

		BeforeEach(func() {
			listSubvolumesOutput = fmt.Sprintf(`ID 257 gen 9 top level 5 path relative/path/to/%s
ID 258 gen 9 top level 257 path relative/path/to/%s/subvolume1
ID 259 gen 9 top level 257 path relative/path/to/%s/subvolume2
`, layerId, layerId, layerId)
		})

		It("deletes the subvolume", func() {
			Expect(cleaner.Clean(layerId)).To(Succeed())
			Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "btrfs",
				Args: []string{"subvolume", "delete", subvolume1},
			}))
			Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
				Path: "btrfs",
				Args: []string{"subvolume", "delete", subvolume2},
			}))
		})

		It("deletes the subvolume directory contents before deleting the subvolume", func() {
			runner.WhenRunning(fake_command_runner.CommandSpec{
				Path: "btrfs",
				Args: []string{"subvolume", "delete", subvolume1},
			}, func(cmd *exec.Cmd) error {
				Expect(removedDirectories).To(ConsistOf(subvolume1))
				return nil
			})

			Expect(cleaner.Clean(layerId)).To(Succeed())
		})

		PIt("when deleting dir contents fails", func() {})

		Context("and the nested subvolumes have nested subvolumes", func() {
			BeforeEach(func() {
				listSubvolumesOutput = fmt.Sprintf(`ID 257 gen 9 top level 5 path relative/path/to/%s
ID 258 gen 9 top level 257 path relative/path/to/%s/subvolume1
ID 259 gen 9 top level 257 path relative/path/to/%s/subvolume1/subsubvol1
`, layerId, layerId, layerId)
			})

			It("deletes the subvolumes deepest-first", func() {
				Expect(cleaner.Clean(layerId)).To(Succeed())
				Expect(runner).To(HaveExecutedSerially(fake_command_runner.CommandSpec{
					Path: "btrfs",
					Args: []string{"subvolume", "delete", subvolume1 + "/subsubvol1"},
				}, fake_command_runner.CommandSpec{
					Path: "btrfs",
					Args: []string{"subvolume", "delete", subvolume1},
				}))
			})
		})
	})

	Context("when running a command fails", func() {
		BeforeEach(func() {
			listSubVolumeErr = errors.New("listing subvolumes failed!")
		})

		It("returns the same error", func() {
			Expect(cleaner.Clean(layerId)).To(MatchError("listing subvolumes failed!"))
		})
	})

	Context("when graphdriver fails to get layer path", func() {
		BeforeEach(func() {
			graphDriverErr = errors.New("graphdriver fail!")
		})

		It("returns the same error", func() {
			Expect(cleaner.Clean(layerId)).To(MatchError("graphdriver fail!"))
		})
	})
})
