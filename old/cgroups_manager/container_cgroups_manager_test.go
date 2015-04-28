package cgroups_manager_test

import (
	"io/ioutil"
	"os"
	"path"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager"
)

var _ = Describe("Container cgroups", func() {
	var cgroupsPath string
	var cgroupsManager *cgroups_manager.ContainerCgroupsManager

	BeforeEach(func() {
		tmpdir, err := ioutil.TempDir(os.TempDir(), "some-cgroups")
		Expect(err).ToNot(HaveOccurred())

		cgroupsPath = tmpdir

		cgroupsManager = cgroups_manager.New(cgroupsPath, "some-container-id")
	})

	Describe("setup cgroups", func() {
		It("creates a cgroups directory for each subsystem", func() {
			err := cgroupsManager.Setup("memory", "cpu")
			Expect(err).ToNot(HaveOccurred())

			Expect(path.Join(cgroupsPath, "memory", "instance-some-container-id")).To(BeADirectory())
			Expect(path.Join(cgroupsPath, "cpu", "instance-some-container-id")).To(BeADirectory())
		})

		Context("when the subsystems contain cpuset", func() {
			It("initializes the cpuset.cpus and cpuset.mems based on the system cgroup", func() {
				Expect(os.MkdirAll(path.Join(cgroupsPath, "cpuset"), 0755)).To(Succeed())

				Expect(ioutil.WriteFile(path.Join(cgroupsPath, "cpuset", "cpuset.cpus"), []byte("the cpuset cpus"), 0600)).To(Succeed())
				Expect(ioutil.WriteFile(path.Join(cgroupsPath, "cpuset", "cpuset.mems"), []byte("the cpuset mems"), 0600)).To(Succeed())

				err := cgroupsManager.Setup("memory", "cpu", "cpuset")
				Expect(err).ToNot(HaveOccurred())

				Expect(ioutil.ReadFile(path.Join(
					cgroupsPath,
					"cpuset",
					"instance-some-container-id",
					"cpuset.cpus"))).
					To(Equal([]byte("the cpuset cpus")))

				Expect(ioutil.ReadFile(path.Join(
					cgroupsPath,
					"cpuset",
					"instance-some-container-id",
					"cpuset.mems"))).
					To(Equal([]byte("the cpuset mems")))
			})
		})
	})

	Describe("adding a process", func() {
		It("writes the process to the tasks file", func() {
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")
			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = cgroupsManager.Add(1235, "memory")
			Expect(err).ToNot(HaveOccurred())

			err = cgroupsManager.Add(1237, "memory")
			Expect(err).ToNot(HaveOccurred())

			value, err := ioutil.ReadFile(path.Join(containerMemoryCgroupsPath, "tasks"))
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.Split(string(value), "\n")).To(ContainElement("1235"))
			Expect(strings.Split(string(value), "\n")).To(ContainElement("1237"))
		})

		Context("when multiple subsystem are provided", func() {
			It("writes the process to task file for each subsystem", func() {
				containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")
				err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
				Expect(err).ToNot(HaveOccurred())

				containerCpuCgroupsPath := path.Join(cgroupsPath, "cpu", "instance-some-container-id")
				err = os.MkdirAll(containerCpuCgroupsPath, 0755)
				Expect(err).ToNot(HaveOccurred())

				err = cgroupsManager.Add(1235, "memory", "cpu")
				Expect(err).ToNot(HaveOccurred())

				value, err := ioutil.ReadFile(path.Join(containerMemoryCgroupsPath, "tasks"))
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.Split(string(value), "\n")).To(ContainElement("1235"))

				value, err = ioutil.ReadFile(path.Join(containerCpuCgroupsPath, "tasks"))
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.Split(string(value), "\n")).To(ContainElement("1235"))
			})
		})
	})

	Describe("setting", func() {
		It("writes the value to the name under the subsytem", func() {
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")
			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = cgroupsManager.Set("memory", "memory.limit_in_bytes", "42")
			Expect(err).ToNot(HaveOccurred())

			value, err := ioutil.ReadFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"))
			Expect(err).ToNot(HaveOccurred())
			Expect(string(value)).To(Equal("42"))
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
	})

	Describe("getting", func() {
		It("reads the current value from the name under the subsystem", func() {
			containerMemoryCgroupsPath := path.Join(cgroupsPath, "memory", "instance-some-container-id")

			err := os.MkdirAll(containerMemoryCgroupsPath, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = ioutil.WriteFile(path.Join(containerMemoryCgroupsPath, "memory.limit_in_bytes"), []byte("123\n"), 0644)
			Expect(err).ToNot(HaveOccurred())

			val, err := cgroupsManager.Get("memory", "memory.limit_in_bytes")
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("123"))
		})
	})

	Describe("retrieving a subsystem path", func() {
		It("returns <path>/<subsytem>/instance-<container-id>", func() {
			Expect(cgroupsManager.SubsystemPath("memory")).To(Equal(
				path.Join(cgroupsPath, "memory", "instance-some-container-id"),
			))

		})
	})
})
