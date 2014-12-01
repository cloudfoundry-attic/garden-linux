package bind_mount_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A container", func() {
	var (
		privilegedContainer bool
		bindMounts          []api.BindMount
		container           api.Container
		containerCreateErr  error
		bmNetworkMask       string
	)

	checkFileAccess := func(readOnly bool, dstPath string, fileName string) {
		// can we read a file?
		filePath := filepath.Join(dstPath, fileName)

		process, err := container.Run(api.ProcessSpec{
			Path: "cat",
			Args: []string{filePath},
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(0))

		// try to write a new file
		filePath = filepath.Join(dstPath, "checkFileAccess-file")

		process, err = container.Run(api.ProcessSpec{
			Path: "touch",
			Args: []string{filePath},
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		if readOnly {
			Ω(process.Wait()).ShouldNot(Equal(0))
		} else {
			Ω(process.Wait()).Should(Equal(0))
		}

		// try to delete an existing file
		filePath = filepath.Join(dstPath, fileName)

		process, err = container.Run(api.ProcessSpec{
			Path: "rm",
			Args: []string{filePath},
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		if readOnly {
			Ω(process.Wait()).ShouldNot(Equal(0))
		} else {
			Ω(process.Wait()).Should(Equal(0))
		}
	}

	testContainerPrivileges := func(readOnly bool, srcPath, dstPath string) {

		createContainerTestFileIn := func(dir string) string {
			fileName := "bind-mount-test-file"
			filePath := filepath.Join(dir, fileName)
			process, err := container.Run(api.ProcessSpec{
				Path: "touch",
				Args: []string{filePath},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).Should(Equal(0))
			return fileName
		}

		Context("and with privileged=true", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			It("is successfully created with correct privileges", func() {
				Ω(containerCreateErr).ShouldNot(HaveOccurred())
				testFileName := createContainerTestFileIn(srcPath)
				checkFileAccess(readOnly, dstPath, testFileName)
			})
		})

		Context("and with privileged=false", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			It("is successfully created with correct privileges", func() {
				Ω(containerCreateErr).ShouldNot(HaveOccurred())
				testFileName := createContainerTestFileIn(srcPath)
				checkFileAccess(readOnly, dstPath, testFileName)
			})
		})
	}

	testHostPrivileges := func(readOnly bool, srcPath, dstPath string) {

		createHostTestFileIn := func(dir string) string {
			fileName := fmt.Sprintf("bind-mount-%d-test-file", GinkgoParallelNode())
			file, err := os.Create(filepath.Join(dir, fileName))
			Ω(err).ShouldNot(HaveOccurred())
			Ω(file.Close()).ShouldNot(HaveOccurred())
			return fileName
		}

		Context("and with privileged=true", func() {
			var testFileName string
			BeforeEach(func() {
				testFileName = createHostTestFileIn(srcPath)
				privilegedContainer = true
			})

			AfterEach(func() {
				os.RemoveAll(testFileName)
			})

			It("is successfully created with correct privileges", func() {
				Ω(containerCreateErr).ShouldNot(HaveOccurred())
				checkFileAccess(readOnly, dstPath, testFileName)
			})
		})

		Context("and with privileged=false", func() {
			var testFileName string
			BeforeEach(func() {
				testFileName = createHostTestFileIn(srcPath)
				privilegedContainer = false
			})

			AfterEach(func() {
				os.RemoveAll(testFileName)
			})

			It("is successfully created with correct privileges", func() {
				Ω(containerCreateErr).ShouldNot(HaveOccurred())
				checkFileAccess(readOnly, dstPath, testFileName)
			})
		})
	}

	BeforeEach(func() {
		bmNetworkMask = "10.0.%d.0/24"
		privilegedContainer = false
		bindMounts = nil
		container = nil
		containerCreateErr = nil
	})

	AfterEach(func() {
		if container != nil {
			err := gardenClient.Destroy(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	JustBeforeEach(func() {
		bmNetwork := fmt.Sprintf(bmNetworkMask, GinkgoParallelNode())
		cspec :=
			api.ContainerSpec{
				Privileged: privilegedContainer,
				BindMounts: bindMounts,
				Network:    bmNetwork,
			}
		container, containerCreateErr = gardenClient.Create(cspec)
		Ω(containerCreateErr).ShouldNot(HaveOccurred())
	})

	Context("with a read-only host bind mount", func() {
		srcPath := "/tmp"
		dstPath := "/home/vcap/readonly"
		BeforeEach(func() {
			bindMounts = []api.BindMount{api.BindMount{
				SrcPath: srcPath,
				DstPath: dstPath,
				Mode:    api.BindMountModeRO,
				Origin:  api.BindMountOriginHost,
			}}
		})
		testHostPrivileges(true, srcPath, dstPath) // == read-only
	})

	Context("with a read-write host bind mount", func() {
		srcPath := "/tmp"
		dstPath := "/home/vcap/readwrite"
		BeforeEach(func() {
			bindMounts = []api.BindMount{api.BindMount{
				SrcPath: srcPath,
				DstPath: dstPath,
				Mode:    api.BindMountModeRW,
				Origin:  api.BindMountOriginHost,
			}}
		})
		testHostPrivileges(false, srcPath, dstPath) // == read-write
	})

	Context("with a read-only container bind mount", func() {
		srcPath := "/home/vcap"
		dstPath := "/home/vcap/readonly"
		BeforeEach(func() {
			bindMounts = []api.BindMount{api.BindMount{
				SrcPath: srcPath,
				DstPath: dstPath,
				Mode:    api.BindMountModeRO,
				Origin:  api.BindMountOriginContainer,
			}}
		})

		testContainerPrivileges(true, srcPath, dstPath) // == read-only
	})

	Context("with a read-write container bind mount", func() {
		srcPath := "/home/vcap"
		dstPath := "/home/vcap/readwrite"
		BeforeEach(func() {
			bindMounts = []api.BindMount{api.BindMount{
				SrcPath: srcPath,
				DstPath: dstPath,
				Mode:    api.BindMountModeRW,
				Origin:  api.BindMountOriginContainer,
			}}
		})

		testContainerPrivileges(false, srcPath, dstPath) // == read-write
	})

})
