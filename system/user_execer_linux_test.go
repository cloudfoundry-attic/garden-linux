package system_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("UserExecer", func() {
	var id uint32

	BeforeEach(func() {
		id = uint32(100000 + GinkgoParallelNode())
		groupAddCmd := exec.Command("groupadd", "-g", fmt.Sprintf("%d", id), "banana")
		groupAddCmd.Stdout = GinkgoWriter
		groupAddCmd.Stderr = GinkgoWriter
		groupAddCmd.Run()

		userAddCmd := exec.Command("useradd", "-g", fmt.Sprintf("%d", id), "-u", fmt.Sprintf("%d", id), "banana")
		userAddCmd.Stdout = GinkgoWriter
		userAddCmd.Stderr = GinkgoWriter
		userAddCmd.Run()
	})

	AfterEach(func() {
		exec.Command("userdel", "banana").Run()
	})

	It("execs a process as specified user", func() {
		out := gbytes.NewBuffer()
		runningTest, err := gexec.Start(
			exec.Command(testUserExecerPath,
				fmt.Sprintf("-uid=%d", id),
				fmt.Sprintf("-gid=%d", id),
				fmt.Sprintf("-workDir=%s", "/tmp")),
			io.MultiWriter(GinkgoWriter, out),
			io.MultiWriter(GinkgoWriter, out),
		)
		Expect(err).NotTo(HaveOccurred())
		runningTest.Wait()

		Expect(runningTest.ExitCode()).To(Equal(0))
		Expect(string(out.Contents())).To(Equal(fmt.Sprintf("%d\n%d\n", id, id)))
	})

	Describe("Working directory", func() {
		var workDir string

		Context("when working directory does not exist", func() {
			BeforeEach(func() {
				workDir = filepath.Join("/tmp", fmt.Sprintf("user-execer-%d", GinkgoParallelNode()))
				_, err := os.Stat(workDir)
				Expect(err).To(HaveOccurred())
				Expect(os.IsNotExist(err)).To(BeTrue())
			})

			AfterEach(func() {
				os.RemoveAll(workDir)
			})

			It("creates working directory", func() {
				runningTest, err := gexec.Start(
					exec.Command(testUserExecerPath,
						fmt.Sprintf("-uid=%d", id),
						fmt.Sprintf("-gid=%d", id),
						fmt.Sprintf("-workDir=%s", workDir)),
					GinkgoWriter,
					GinkgoWriter,
				)
				Expect(err).NotTo(HaveOccurred())
				runningTest.Wait()
				Expect(runningTest.ExitCode()).To(Equal(0))

				info, err := os.Stat(workDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(info.IsDir()).To(BeTrue())

				stats := info.Sys().(*syscall.Stat_t)
				Expect(stats.Uid).To(Equal(id))
				Expect(stats.Gid).To(Equal(id))
			})

			Context("when the user has NOT permissiongs", func() {
				BeforeEach(func() {
					workDir = "/root/nonexist"
				})

				It("failes to create working directory because of permissions", func() {
					out := gbytes.NewBuffer()
					cmd := exec.Command(testUserExecerPath,
						fmt.Sprintf("-uid=%d", id),
						fmt.Sprintf("-gid=%d", id),
						fmt.Sprintf("-workDir=%s", workDir))

					runningTest, _ := gexec.Start(cmd, GinkgoWriter, out)
					runningTest.Wait()
					Expect(runningTest.ExitCode()).ToNot(Equal(0))
					Expect(out).To(gbytes.Say(fmt.Sprintf("system: mkdir %s: permission denied", workDir)))
				})
			})
		})

		Context("when working directory is not provided", func() {
			JustBeforeEach(func() {
				workDir = ""
			})

			It("fails to execute", func() {
				out := gbytes.NewBuffer()
				cmd := exec.Command(testUserExecerPath,
					fmt.Sprintf("-uid=%d", id),
					fmt.Sprintf("-gid=%d", id),
					fmt.Sprintf("-workDir=%s", workDir))

				runningTest, _ := gexec.Start(cmd, GinkgoWriter, out)
				runningTest.Wait()
				Expect(runningTest.ExitCode()).ToNot(Equal(0))
				Expect(out).To(gbytes.Say("system: working directory is not provided"))
			})
		})

		Context("when working directory does exist", func() {
			JustBeforeEach(func() {
				workDir = "/tmp"
			})

			Context("when the user has permissions to run in working directory", func() {
				It("returns exit status 0 (succeeds)", func() {
					runningTest, err := gexec.Start(
						exec.Command(testUserExecerPath,
							fmt.Sprintf("-uid=%d", id),
							fmt.Sprintf("-gid=%d", id),
							fmt.Sprintf("-workDir=%s", workDir)),
						GinkgoWriter,
						GinkgoWriter,
					)
					Expect(err).NotTo(HaveOccurred())
					runningTest.Wait()
					Expect(runningTest.ExitCode()).To(Equal(0))
				})
			})

			Context("when the user has NOT permissions to run in working directory", func() {
				JustBeforeEach(func() {
					workDir = "/root"
				})

				It("fails to execute", func() {
					out := gbytes.NewBuffer()
					cmd := exec.Command(testUserExecerPath,
						fmt.Sprintf("-uid=%d", id),
						fmt.Sprintf("-gid=%d", id),
						fmt.Sprintf("-workDir=%s", workDir))

					runningTest, _ := gexec.Start(cmd, GinkgoWriter, out)
					runningTest.Wait()
					Expect(runningTest.ExitCode()).ToNot(Equal(0))
					Expect(out).To(gbytes.Say(fmt.Sprintf("system: invalid working directory: %s", workDir)))
				})
			})
		})
	})
})
