package cgroups_manager_test

import (
	"io/ioutil"
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/cgroups_manager"
)

var _ = Describe("Container cgroups", func() {
	var cgroupsPath string
	var cgroupsManager *cgroups_manager.ContainerCgroupsManager

	BeforeEach(func() {
		tmpdir, err := ioutil.TempDir(os.TempDir(), "some-cgroups")
		Ω(err).ShouldNot(HaveOccurred())

		cgroupsPath = tmpdir

		cgroupsManager = cgroups_manager.New(cgroupsPath, "some-container-id")
	})

	Describe("setting", func() {
		It("writes the value to the name under the subsytem", func() {
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")
			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Ω(err).ShouldNot(HaveOccurred())

			err = cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
			Ω(err).ShouldNot(HaveOccurred())

			value, err := ioutil.ReadFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"))
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(value)).Should(Equal("42"))
		})

		Context("when the cgroups directory does not exist", func() {
			BeforeEach(func() {
				err := os.RemoveAll(cgroupsPath)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("returns an error", func() {
				err := cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
				Ω(err).Should(HaveOccurred())
			})
		})
	})

	Describe("getting", func() {
		It("reads the current value from the name under the subsystem", func() {
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")

			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Ω(err).ShouldNot(HaveOccurred())

			err = ioutil.WriteFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"), []byte("123\n"), 0644)
			Ω(err).ShouldNot(HaveOccurred())

			val, err := cgroupsManager.Get("memory", "memory.limit_in_bytes")
			Ω(err).ShouldNot(HaveOccurred())
			Ω(val).Should(Equal("123"))
		})
	})

	Describe("retrieving a subsystem path", func() {
		It("returns <path>/<subsytem>/instance-<container-id>", func() {
			Ω(cgroupsManager.SubsystemPath("memory")).Should(Equal(
				path.Join(cgroupsPath, "memory", "instance-some-container-id"),
			))

		})
	})
})
