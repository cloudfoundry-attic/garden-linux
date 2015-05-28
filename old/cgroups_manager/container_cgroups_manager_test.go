package cgroups_manager_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager/fake_cgroup_reader"
)

var _ = Describe("Container cgroups", func() {
	var (
		cgroupsPath    string
		cgroupsManager *cgroups_manager.ContainerCgroupsManager
		cgroupReader   *fake_cgroup_reader.FakeCgroupReader
	)

	BeforeEach(func() {
		cgroupReader = new(fake_cgroup_reader.FakeCgroupReader)
		tmpdir, err := ioutil.TempDir(os.TempDir(), "some-cgroups")
		Expect(err).ToNot(HaveOccurred())

		cgroupsPath = tmpdir

		cgroupsManager = cgroups_manager.New(cgroupsPath, "some-container-id", cgroupReader)
	})

	Describe("setting", func() {
		It("writes the value to the name under the subsytem", func() {
			cgroupReader.CgroupNodeReturns("/somenestedpath3/somenestedpath4", nil)
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "somenestedpath3", "somenestedpath4", "instance-some-container-id")
			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
			Expect(err).ToNot(HaveOccurred())

			value, err := ioutil.ReadFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"))
			Expect(err).ToNot(HaveOccurred())
			Expect(string(value)).To(Equal("42"))

			Expect(cgroupReader.CgroupNodeArgsForCall(0)).To(Equal("memory"))
		})

		Context("when the cgroups directory does not exist", func() {
			BeforeEach(func() {
				err := os.RemoveAll(cgroupsPath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns an error", func() {
				err := cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the cgroup node is not found", func() {
			It("returns an error", func() {
				cgroupReader.CgroupNodeReturns("", errors.New("banana"))

				err := cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
				Expect(err).To(MatchError(ContainSubstring("banana")))
			})
		})
	})

	Describe("getting", func() {
		It("reads the current value from the name under the subsystem", func() {
			cgroupReader.CgroupNodeReturns("/somenestedpath/somenestedpath2", nil)
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "somenestedpath", "somenestedpath2", "instance-some-container-id")

			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = ioutil.WriteFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"), []byte("123\n"), 0644)
			Expect(err).ToNot(HaveOccurred())

			val, err := cgroupsManager.Get("memory", "memory.limit_in_bytes")
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("123"))

			Expect(cgroupReader.CgroupNodeArgsForCall(0)).To(Equal("memory"))
		})

		Context("when the cgroup node is not found", func() {
			It("returns an error", func() {
				cgroupReader.CgroupNodeReturns("", errors.New("pineapple"))

				_, err := cgroupsManager.Get("memory", "memory.limit_in_bytes")
				Expect(err).To(MatchError(ContainSubstring("pineapple")))
			})
		})
	})

	Describe("retrieving a subsystem path", func() {
		It("returns <path>/<subsytem>/instance-<container-id>", func() {
			Expect(cgroupsManager.SubsystemPath("memory")).To(Equal(
				path.Join(cgroupsPath, "memory", "instance-some-container-id"),
			))
		})

		Context("when there is an error reading cgroup node path", func() {

			It("returns an error", func() {
				cgroupReader.CgroupNodeReturns("", errors.New("o no!"))
				_, err := cgroupsManager.SubsystemPath("memory")
				Expect(err).To(MatchError("o no!"))
			})
		})
	})
})
