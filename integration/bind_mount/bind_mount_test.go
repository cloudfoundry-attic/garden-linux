package bind_mount_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container", func() {
	var (
		container          garden.Container
		containerCreateErr error

		// container create parms
		privilegedContainer bool
		srcPath             string                 // bm: source
		dstPath             string                 // bm: destination
		bindMountMode       garden.BindMountMode   // bm: RO or RW
		bindMountOrigin     garden.BindMountOrigin // bm: Container or Host

		// pre-existing file for permissions testing
		testFileName string
	)

	BeforeEach(func() {
		privilegedContainer = false
		container = nil
		containerCreateErr = nil
		srcPath = ""
		dstPath = ""
		bindMountMode = garden.BindMountModeRO
		bindMountOrigin = garden.BindMountOriginHost
		testFileName = ""
	})

	JustBeforeEach(func() {
		container, containerCreateErr = gardenClient.Create(
			garden.ContainerSpec{
				Privileged: privilegedContainer,
				BindMounts: []garden.BindMount{garden.BindMount{
					SrcPath: srcPath,
					DstPath: dstPath,
					Mode:    bindMountMode,
					Origin:  bindMountOrigin,
				}},
				Network: fmt.Sprintf("10.0.%d.0/24", GinkgoParallelNode()),
			})
	})

	AfterEach(func() {
		if container != nil {
			err := gardenClient.Destroy(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	Context("with an invalid source directory", func() {
		BeforeEach(func() {
			srcPath = "/does-not-exist"
			dstPath = "/home/vcap/should-not-be-created"
		})

		It("should fail to be created", func() {
			Ω(containerCreateErr).Should(HaveOccurred())
		})
	})

	Context("with a host origin bind-mount", func() {
		BeforeEach(func() {
			srcPath, testFileName = createTestHostDirAndTestFile()
			bindMountOrigin = garden.BindMountOriginHost
		})

		AfterEach(func() {
			err := os.RemoveAll(srcPath)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("which is read-only", func() {
			BeforeEach(func() {
				bindMountMode = garden.BindMountModeRO
				dstPath = "/home/vcap/readonly"
			})

			Context("and with privileged=true", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})

			Context("and with privileged=false", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})
		})

		Context("which is read-write", func() {
			BeforeEach(func() {
				bindMountMode = garden.BindMountModeRW
				dstPath = "/home/vcap/readwrite"
			})

			Context("and with privileged=true", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})

			Context("and with privileged=false", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})
		})
	})

	Context("with a container origin bind-mount", func() {
		BeforeEach(func() {
			srcPath = "/home/vcap"
			bindMountOrigin = garden.BindMountOriginContainer
		})

		JustBeforeEach(func() {
			testFileName = createContainerTestFileIn(container, srcPath)
		})

		Context("which is read-only", func() {
			BeforeEach(func() {
				bindMountMode = garden.BindMountModeRO
				dstPath = "/home/vcap/readonly"
			})

			Context("and with privileged=true", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})

			Context("and with privileged=false", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})

		})

		Context("which is read-write", func() {
			BeforeEach(func() {
				bindMountMode = garden.BindMountModeRW
				dstPath = "/home/vcap/readwrite"
			})

			Context("and with privileged=true", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})

			Context("and with privileged=false", func() {
				BeforeEach(func() {
					privilegedContainer = false
				})

				It("is successfully created with correct privileges for non-root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, false)
				})

				It("is successfully created with correct privileges for root in container", func() {
					Ω(containerCreateErr).ShouldNot(HaveOccurred())
					checkFileAccess(container, bindMountMode, bindMountOrigin, dstPath, testFileName, privilegedContainer, true)
				})
			})
		})
	})
})

func createTestHostDirAndTestFile() (string, string) {
	tstHostDir, err := ioutil.TempDir("", "bind-mount-test-dir")
	Ω(err).ShouldNot(HaveOccurred())
	err = os.Chown(tstHostDir, 0, 0)
	Ω(err).ShouldNot(HaveOccurred())
	err = os.Chmod(tstHostDir, 0755)
	Ω(err).ShouldNot(HaveOccurred())

	fileName := fmt.Sprintf("bind-mount-%d-test-file", GinkgoParallelNode())
	file, err := os.OpenFile(filepath.Join(tstHostDir, fileName), os.O_CREATE|os.O_RDWR, 0777)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(file.Close()).ShouldNot(HaveOccurred())

	return tstHostDir, fileName
}

func createContainerTestFileIn(container garden.Container, dir string) string {
	fileName := "bind-mount-test-file"
	filePath := filepath.Join(dir, fileName)

	process, err := container.Run(garden.ProcessSpec{
		Path:       "touch",
		Args:       []string{filePath},
		Privileged: true,
	}, garden.ProcessIO{nil, os.Stdout, os.Stderr})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0))

	process, err = container.Run(garden.ProcessSpec{
		Path:       "chmod",
		Args:       []string{"0777", filePath},
		Privileged: true,
	}, garden.ProcessIO{nil, os.Stdout, os.Stderr})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0))

	return fileName
}

func checkFileAccess(container garden.Container, bindMountMode garden.BindMountMode, bindMountOrigin garden.BindMountOrigin, dstPath string, fileName string, privCtr, privReq bool) {
	readOnly := (garden.BindMountModeRO == bindMountMode)
	ctrOrigin := (garden.BindMountOriginContainer == bindMountOrigin)
	realRoot := (privReq && privCtr)

	// can we read a file?
	filePath := filepath.Join(dstPath, fileName)

	process, err := container.Run(garden.ProcessSpec{
		Path:       "cat",
		Args:       []string{filePath},
		Privileged: privReq,
	}, garden.ProcessIO{})
	Ω(err).ShouldNot(HaveOccurred())

	Ω(process.Wait()).Should(Equal(0))

	// try to write a new file
	filePath = filepath.Join(dstPath, "checkFileAccess-file")

	process, err = container.Run(garden.ProcessSpec{
		Path:       "touch",
		Args:       []string{filePath},
		Privileged: privReq,
	}, garden.ProcessIO{})
	Ω(err).ShouldNot(HaveOccurred())

	if readOnly || (!realRoot && !ctrOrigin) {
		Ω(process.Wait()).ShouldNot(Equal(0))
	} else {
		Ω(process.Wait()).Should(Equal(0))
	}

	// try to delete an existing file
	filePath = filepath.Join(dstPath, fileName)

	process, err = container.Run(garden.ProcessSpec{
		Path:       "rm",
		Args:       []string{filePath},
		Privileged: privReq,
	}, garden.ProcessIO{})
	Ω(err).ShouldNot(HaveOccurred())
	if readOnly || (!realRoot && !ctrOrigin) {
		Ω(process.Wait()).ShouldNot(Equal(0))
	} else {
		Ω(process.Wait()).Should(Equal(0))
	}
}
