package cgroups_manager_test

import (
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CgroupReader", func() {

	Context("when the file exists", func() {
		Context("and is in the correct format", func() {
			Context("when the requested subsystem exists", func() {
				It("returns the cgroup node for the subsystem", func() {
					reader := cgroups_manager.LinuxCgroupReader{
						Path: filepath.Join("test_assets", "proc_self_cgroup.txt"),
					}

					cpuNode, err := reader.CgroupNode("cpu")
					Expect(err).ToNot(HaveOccurred())
					Expect(cpuNode).To(Equal("/somedir"))

					memoryNode, err := reader.CgroupNode("memory")
					Expect(err).ToNot(HaveOccurred())
					Expect(memoryNode).To(Equal("/somedir/mem"))
				})
			})

			Context("when the requested subsystem does not exist", func() {

				It("returns an error", func() {
					reader := cgroups_manager.LinuxCgroupReader{
						Path: filepath.Join("test_assets", "proc_self_cgroup.txt"),
					}
					_, err := reader.CgroupNode("oi")
					Expect(err).To(MatchError(ContainSubstring("requested subsystem oi does not exist")))
				})
			})
		})

		Context("and is not in the correct format", func() {

			It("returns an error", func() {
				reader := cgroups_manager.LinuxCgroupReader{
					Path: filepath.Join("test_assets", "proc_self_cgroup_invalid.txt"),
				}

				_, err := reader.CgroupNode("cpu")
				Expect(err).To(HaveOccurred())

				_, err = reader.CgroupNode("devices")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("when the file does not exist", func() {
		It("returns an error", func() {
			reader := cgroups_manager.LinuxCgroupReader{
				Path: filepath.Join("non_existing_dir", "non_existing_file.txt"),
			}

			_, err := reader.CgroupNode("cpu")
			Expect(err).To(HaveOccurred())
		})
	})
})
