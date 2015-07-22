package lifecycle_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Creating a container", func() {
	Describe("Overlapping networks", func() {
		Context("when the requested Network overlaps the dynamic allocation range", func() {
			It("returns an error message naming the overlapped range", func() {
				client = startGarden("--networkPool", "1.2.3.0/24")
				_, err := client.Create(garden.ContainerSpec{Network: "1.2.3.0/25"})
				Expect(err).To(MatchError("the requested subnet (1.2.3.0/25) overlaps the dynamic allocation range (1.2.3.0/24)"))
			})
		})

		Context("when the requested Network overlaps another subnet", func() {
			It("returns an error message naming the overlapped range", func() {
				client = startGarden()
				_, err := client.Create(garden.ContainerSpec{Privileged: false, Network: "10.2.0.0/29"})
				Expect(err).ToNot(HaveOccurred())
				_, err = client.Create(garden.ContainerSpec{Privileged: false, Network: "10.2.0.0/30"})
				Expect(err).To(MatchError("the requested subnet (10.2.0.0/30) overlaps an existing subnet (10.2.0.0/29)"))
			})
		})
	})

	Describe("concurrent creation of containers based on same docker rootfs", func() {

		It("retains the full rootFS without truncating files", func() {
			client = startGarden()
			c1chan := make(chan garden.Container)
			c2chan := make(chan garden.Container)
			c3chan := make(chan garden.Container)

			createContainer := func(ch chan<- garden.Container) {
				defer GinkgoRecover()
				c, err := client.Create(garden.ContainerSpec{RootFSPath: "docker:///cloudfoundry/large_layers", Privileged: false})
				Expect(err).ToNot(HaveOccurred())
				ch <- c
			}

			runInContainer := func(c garden.Container) {
				out := gbytes.NewBuffer()
				process, err := c.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "/usr/local/go/bin/go",
					Args: []string{"version"},
				}, garden.ProcessIO{
					Stdout: out,
					Stderr: out,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(Equal(0))
				Eventually(out).Should(gbytes.Say("go version go1.4.2 linux/amd64"))
			}

			go createContainer(c1chan)
			go createContainer(c2chan)
			go createContainer(c3chan)

			runInContainer(<-c1chan)
			runInContainer(<-c2chan)
			runInContainer(<-c3chan)
		})
	})

	Describe("concurrently creating", func() {
		It("does not deadlock", func() {
			client = startGarden()
			wg := new(sync.WaitGroup)

			errors := make(chan error, 50)
			for i := 0; i < 40; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()

					container, err := client.Create(garden.ContainerSpec{})
					if err != nil {
						errors <- err
					} else {
						client.Destroy(container.Handle())
					}

					wg.Done()
				}()
			}
			wg.Wait()

			Expect(errors).ToNot(Receive())
		})
	})

	Describe("concurrently destroying", func() {
		allBridges := func() []byte {
			stdout := gbytes.NewBuffer()
			cmd, err := gexec.Start(exec.Command("ip", "a"), stdout, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			cmd.Wait()

			return stdout.Contents()
		}

		It("does not leave residual bridges", func() {
			client = startGarden()

			bridgePrefix := fmt.Sprintf("w%db-", GinkgoParallelNode())
			Expect(allBridges()).ToNot(ContainSubstring(bridgePrefix))

			handles := make([]string, 0)
			for i := 0; i < 5; i++ {
				c, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				handles = append(handles, c.Handle())
			}

			retry := func(fn func() error) error {
				var err error
				for retry := 0; retry < 3; retry++ {
					err = fn()
					if err == nil {
						break
					}
				}
				return err
			}

			wg := new(sync.WaitGroup)
			errors := make(chan error, 50)
			for _, h := range handles {
				wg.Add(1)
				go func(h string) {
					err := retry(func() error { return client.Destroy(h) })

					if err != nil {
						errors <- err
					}

					wg.Done()
				}(h)
			}

			wg.Wait()

			Expect(errors).ToNot(Receive())
			Expect(client.Containers(garden.Properties{})).To(HaveLen(0)) // sanity check

			Eventually(allBridges, "60s", "10s").ShouldNot(ContainSubstring(bridgePrefix))
		})
	})

	Context("when the create container fails because of env failure", func() {
		allBridges := func() []byte {
			stdout := gbytes.NewBuffer()
			cmd, err := gexec.Start(exec.Command("ip", "a"), stdout, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			cmd.Wait()
			return stdout.Contents()
		}

		It("does not leave bridges resources around", func() {
			client = startGarden()
			bridgePrefix := fmt.Sprintf("w%db-", GinkgoParallelNode())
			Expect(allBridges()).ToNot(ContainSubstring(bridgePrefix))
			var err error
			_, err = client.Create(garden.ContainerSpec{
				Env: []string{"hello"}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(HavePrefix("process: malformed environment")))
			//check no bridges are leaked
			Eventually(allBridges).ShouldNot(ContainSubstring(bridgePrefix))
		})

		It("does not leave network namespaces resources around", func() {
			client = startGarden()
			var err error
			_, err = client.Create(garden.ContainerSpec{
				Env: []string{"hello"}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(HavePrefix("process: malformed environment")))
			//check no network namespaces are leaked
			stdout := gbytes.NewBuffer()
			cmd, err := gexec.Start(
				exec.Command(
					"sh",
					"-c",
					"mount -n -t tmpfs tmpfs /sys && ip netns list && umount /sys",
				),
				stdout,
				GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(cmd.Wait("1s").ExitCode()).To(Equal(0))
			Expect(stdout.Contents()).To(Equal([]byte{}))
		})

	})

	Context("when the container fails to start", func() {
		It("does not leave resources around", func() {
			client = startGarden()
			client.Create(garden.ContainerSpec{
				BindMounts: []garden.BindMount{{
					SrcPath: "fictional",
					DstPath: "whereami",
				}},
			})

			depotDir := filepath.Join(
				os.TempDir(),
				fmt.Sprintf("test-garden-%d", GinkgoParallelNode()),
				"containers",
			)
			Expect(ioutil.ReadDir(depotDir)).To(HaveLen(0))
		})
	})

	Context("when the container is created succesfully", func() {
		var container garden.Container

		var privilegedContainer bool
		var rootfs string

		JustBeforeEach(func() {
			client = startGarden()

			var err error
			container, err = client.Create(garden.ContainerSpec{Privileged: privilegedContainer, RootFSPath: rootfs})
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if container != nil {
				Expect(client.Destroy(container.Handle())).To(Succeed())
			}
		})

		BeforeEach(func() {
			privilegedContainer = false
			rootfs = ""
		})

		It("sources /etc/seed", func() {
			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "test",
				Args: []string{"-e", "/tmp/ran-seed"},
			},
				garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())

			if exitStatus != 0 {
				Fail(fmt.Sprintf(
					"Non zero exit status %d:\n stderr says: %s\n stdout says: %s\n",
					exitStatus,
					string(stderr.Contents()),
					string(stdout.Contents()),
				))
			}
			Expect(exitStatus).To(Equal(0))
		})

		Context("when the rootfs is a symlink", func() {
			var symlinkDir string

			BeforeEach(func() {
				symlinkDir, err := ioutil.TempDir("", "test-symlink")
				Expect(err).ToNot(HaveOccurred())

				rootfs = path.Join(symlinkDir, "rootfs")

				err = os.Symlink(runner.RootFSPath, rootfs)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				os.RemoveAll(symlinkDir)
			})

			It("follows the symlink", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "ls",
					Args: []string{"/"},
				}, garden.ProcessIO{
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(process.Wait()).To(BeZero())

				Expect(stdout).To(gbytes.Say("bin"))
			})
		})

		Context("and running a process", func() {
			Context("when root is requested", func() {
				It("runs as root inside the container", func() {
					stdout := gbytes.NewBuffer()

					_, err := container.Run(garden.ProcessSpec{
						Path: "whoami",
						User: "root",
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: GinkgoWriter,
					})

					Expect(err).ToNot(HaveOccurred())
					Eventually(stdout).Should(gbytes.Say("root\n"))
				})

				Context("and there is no /root directory in the image", func() {
					BeforeEach(func() {
						rootfs = "docker:///onsi/grace-busybox"
					})

					It("still allows running as root", func() {
						_, err := container.Run(garden.ProcessSpec{
							Path: "ls",
							User: "root",
						}, garden.ProcessIO{})

						Expect(err).ToNot(HaveOccurred())
					})
				})

				Context("by default (unprivileged)", func() {
					It("does not get root privileges on host resources", func() {
						process, err := container.Run(garden.ProcessSpec{
							Path: "sh",
							User: "root",
							Args: []string{"-c", "echo h > /proc/sysrq-trigger"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).ToNot(Equal(0))
					})

					It("can write to files in the /root directory", func() {
						process, err := container.Run(garden.ProcessSpec{
							User: "root",
							Path: "sh",
							Args: []string{"-c", `touch /root/potato`},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).To(Equal(0))
					})

					Context("with a docker image", func() {
						BeforeEach(func() {
							rootfs = "docker:///cloudfoundry/preexisting_users"
						})

						It("sees root-owned files in the rootfs as owned by the container's root user", func() {
							stdout := gbytes.NewBuffer()
							process, err := container.Run(garden.ProcessSpec{
								User: "root",
								Path: "sh",
								Args: []string{"-c", `ls -l /sbin | grep -v wsh | grep -v hook`},
							}, garden.ProcessIO{Stdout: stdout})
							Expect(err).ToNot(HaveOccurred())

							Expect(process.Wait()).To(Equal(0))
							Expect(stdout).NotTo(gbytes.Say("nobody"))
							Expect(stdout).NotTo(gbytes.Say("65534"))
							Expect(stdout).To(gbytes.Say(" root "))
						})

						It("sees the /dev/pts and /dev/ptmx as owned by the container's root user", func() {
							stdout := gbytes.NewBuffer()
							process, err := container.Run(garden.ProcessSpec{
								User: "root",
								Path: "sh",
								Args: []string{"-c", "ls -l /dev/pts /dev/ptmx"},
							}, garden.ProcessIO{Stdout: stdout, Stderr: GinkgoWriter})
							Expect(err).ToNot(HaveOccurred())

							Expect(process.Wait()).To(Equal(0))
							Expect(stdout).NotTo(gbytes.Say("nobody"))
							Expect(stdout).NotTo(gbytes.Say("65534"))
							Expect(stdout).To(gbytes.Say(" root "))
						})

						if os.Getenv("BTRFS_SUPPORTED") != "" { // VFS driver does not support this feature`
							It("sees the root directory as owned by the container's root user", func() {
								stdout := gbytes.NewBuffer()
								process, err := container.Run(garden.ProcessSpec{
									User: "root",
									Path: "sh",
									Args: []string{"-c", "ls -al / | head -n 2"},
								}, garden.ProcessIO{Stdout: stdout, Stderr: GinkgoWriter})
								Expect(err).ToNot(HaveOccurred())

								Expect(process.Wait()).To(Equal(0))
								Expect(stdout).NotTo(gbytes.Say("nobody"))
								Expect(stdout).NotTo(gbytes.Say("65534"))
								Expect(stdout).To(gbytes.Say(" root "))
							})
						}

						It("sees alice-owned files as owned by alice", func() {
							stdout := gbytes.NewBuffer()
							process, err := container.Run(garden.ProcessSpec{
								User: "alice",
								Path: "sh",
								Args: []string{"-c", `ls -l /home/alice`},
							}, garden.ProcessIO{Stdout: stdout})
							Expect(err).ToNot(HaveOccurred())

							Expect(process.Wait()).To(Equal(0))
							Expect(stdout).To(gbytes.Say(" alice "))
							Expect(stdout).To(gbytes.Say(" alicesfile"))
						})

						It("sees devices as owned by root", func() {
							out := gbytes.NewBuffer()
							process, err := container.Run(garden.ProcessSpec{
								User: "root",
								Path: "ls",
								Args: []string{"-la", "/dev/tty"},
							}, garden.ProcessIO{
								Stdout: out,
								Stderr: out,
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(process.Wait()).To(Equal(0))
							Expect(string(out.Contents())).To(ContainSubstring(" root "))
							Expect(string(out.Contents())).ToNot(ContainSubstring("nobody"))
							Expect(string(out.Contents())).ToNot(ContainSubstring("65534"))
						})

						It("lets alice write in /home/alice", func() {
							process, err := container.Run(garden.ProcessSpec{
								User: "alice",
								Path: "touch",
								Args: []string{"/home/alice/newfile"},
							}, garden.ProcessIO{})
							Expect(err).ToNot(HaveOccurred())
							Expect(process.Wait()).To(Equal(0))
						})

						It("lets root write to files in the /root directory", func() {
							process, err := container.Run(garden.ProcessSpec{
								User: "root",
								Path: "sh",
								Args: []string{"-c", `touch /root/potato`},
							}, garden.ProcessIO{})
							Expect(err).ToNot(HaveOccurred())
							Expect(process.Wait()).To(Equal(0))
						})

						It("preserves pre-existing dotfiles from base image", func() {
							out := gbytes.NewBuffer()
							process, err := container.Run(garden.ProcessSpec{
								User: "root",
								Path: "cat",
								Args: []string{"/.foo"},
							}, garden.ProcessIO{
								Stdout: out,
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(process.Wait()).To(Equal(0))
							Expect(out).To(gbytes.Say("this is a pre-existing dotfile"))
						})
					})
				})

				Context("when the 'privileged' flag is set on the create call", func() {
					BeforeEach(func() {
						privilegedContainer = true
					})

					It("gets real root privileges", func() {
						process, err := container.Run(garden.ProcessSpec{
							Path: "sh",
							User: "root",
							Args: []string{"-c", "echo h > /proc/sysrq-trigger"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).To(Equal(0))
					})

					It("can write to files in the /root directory", func() {
						process, err := container.Run(garden.ProcessSpec{
							User: "root",
							Path: "sh",
							Args: []string{"-c", `touch /root/potato`},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).To(Equal(0))
					})

					It("sees root-owned files in the rootfs as owned by the container's root user", func() {
						stdout := gbytes.NewBuffer()
						process, err := container.Run(garden.ProcessSpec{
							User: "root",
							Path: "sh",
							Args: []string{"-c", `ls -l /sbin | grep -v wsh | grep -v hook`},
						}, garden.ProcessIO{Stdout: io.MultiWriter(GinkgoWriter, stdout)})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).To(Equal(0))
						Expect(stdout).NotTo(gbytes.Say("nobody"))
						Expect(stdout).NotTo(gbytes.Say("65534"))
						Expect(stdout).To(gbytes.Say(" root "))
					})

					Context("when the process is run as non-root user", func() {
						BeforeEach(func() {
							rootfs = os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS")
						})

						Context("and the user changes to root", func() {
							JustBeforeEach(func() {
								process, err := container.Run(garden.ProcessSpec{
									User: "root",
									Path: "sh",
									Args: []string{"-c", `echo "ALL            ALL = (ALL) NOPASSWD: ALL" >> /etc/sudoers`},
								}, garden.ProcessIO{
									Stdout: GinkgoWriter,
									Stderr: GinkgoWriter,
								})

								Expect(err).ToNot(HaveOccurred())
								Expect(process.Wait()).To(Equal(0))
							})

							It("can chown files", func() {
								process, err := container.Run(garden.ProcessSpec{
									User: "vcap",
									Path: "sudo",
									Args: []string{"chown", "-R", "vcap", "/tmp"},
								}, garden.ProcessIO{
									Stdout: GinkgoWriter,
									Stderr: GinkgoWriter,
								})

								Expect(err).ToNot(HaveOccurred())
								Expect(process.Wait()).To(Equal(0))
							})

							It("does not have certain capabilities", func() {
								// This attempts to set system time which requires the CAP_SYS_TIME permission.
								process, err := container.Run(garden.ProcessSpec{
									User: "vcap",
									Path: "sudo",
									Args: []string{"date", "--set", "+2 minutes"},
								}, garden.ProcessIO{
									Stdout: GinkgoWriter,
									Stderr: GinkgoWriter,
								})

								Expect(err).ToNot(HaveOccurred())
								Expect(process.Wait()).ToNot(Equal(0))
							})
						})
					})
				})
			})

			Measure("it should stream stdout and stderr efficiently", func(b Benchmarker) {
				b.Time("(baseline) streaming 50M of stdout to /dev/null", func() {
					stdout := gbytes.NewBuffer()
					stderr := gbytes.NewBuffer()

					_, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", "tr '\\0' 'a' < /dev/zero | dd count=50 bs=1M of=/dev/null; echo done"},
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: stderr,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout, "2s").Should(gbytes.Say("done\n"))
				})

				time := b.Time("streaming 50M of data via garden", func() {
					stdout := gbytes.NewBuffer()
					stderr := gbytes.NewBuffer()

					_, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", "tr '\\0' 'a' < /dev/zero | dd count=50 bs=1M; echo done"},
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: stderr,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout, "10s").Should(gbytes.Say("done\n"))
				})

				Expect(time.Seconds()).To(BeNumerically("<", 3))
			}, 10)

			It("streams output back and reports the exit status", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "sleep 0.5; echo $FIRST; sleep 0.5; echo $SECOND >&2; sleep 0.5; exit 42"},
					Env:  []string{"FIRST=hello", "SECOND=goodbye"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("hello\n"))
				Eventually(stderr).Should(gbytes.Say("goodbye\n"))
				Expect(process.Wait()).To(Equal(42))
			})

			It("sends a TERM signal to the process if requested", func() {

				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", `
				  trap 'echo termed; exit 42' SIGTERM

					while true; do
					  echo waiting
					  sleep 1
					done
				`},
				}, garden.ProcessIO{
					Stdout: io.MultiWriter(GinkgoWriter, stdout),
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("waiting"))
				Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
				Eventually(stdout, "2s").Should(gbytes.Say("termed"))
				Expect(process.Wait()).To(Equal(42))
			})

			It("sends a TERM signal to the process run by root if requested", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", `
				  trap 'echo termed; exit 42' SIGTERM

					while true; do
					  echo waiting
					  sleep 1
					done
				`},
				}, garden.ProcessIO{
					Stdout: io.MultiWriter(GinkgoWriter, stdout),
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("waiting"))
				Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
				Eventually(stdout, "2s").Should(gbytes.Say("termed"))
				Expect(process.Wait()).To(Equal(42))
			})

			Context("even when /bin/kill does not exist", func() {
				JustBeforeEach(func() {
					process, err := container.Run(garden.ProcessSpec{
						User: "root",
						Path: "rm",
						Args: []string{"/bin/kill"},
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))
				})

				checkProcessIsGone := func(container garden.Container) {
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", `
						 ps ax -o args= | grep -q '^sh -c while'
					 `},
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(1))
					Eventually(stdout).ShouldNot(gbytes.Say("waiting"))
				}

				It("sends a KILL signal to the process if requested", func(done Done) {
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", `
							while true; do
							  echo waiting
								sleep 1
							done
						`},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Eventually(stdout).Should(gbytes.Say("waiting"))

					Expect(process.Signal(garden.SignalKill)).To(Succeed())
					Expect(process.Wait()).To(Equal(255))

					checkProcessIsGone(container)

					close(done)
				}, 10.0)

				It("sends a TERMINATE signal to the process if requested", func(done Done) {
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", `
							while true; do
							  echo waiting
								sleep 1
							done
						`},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Eventually(stdout).Should(gbytes.Say("waiting"))

					Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
					Expect(process.Wait()).To(Equal(255))

					checkProcessIsGone(container)

					close(done)
				}, 10.0)
			})

			It("avoids a race condition when sending a kill signal", func(done Done) {
				for i := 0; i < 100; i++ {
					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", `while true; do echo -n "x"; sleep 1; done`},
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Signal(garden.SignalKill)).To(Succeed())
					Expect(process.Wait()).To(Equal(255))
				}

				close(done)
			}, 480.0)

			It("collects the process's full output, even if it exits quickly after", func() {
				for i := 0; i < 100; i++ {
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "sh",
						Args: []string{"-c", "cat <&0"},
					}, garden.ProcessIO{
						Stdin:  bytes.NewBuffer([]byte("hi stdout")),
						Stderr: os.Stderr,
						Stdout: stdout,
					})

					if err != nil {
						println("ERROR: " + err.Error())
						select {}
					}

					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))

					Expect(stdout).To(gbytes.Say("hi stdout"))
				}
			})

			It("streams input to the process's stdin", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "cat <&0"},
				}, garden.ProcessIO{
					Stdin:  bytes.NewBufferString("hello\nworld"),
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("hello\nworld"))
				Expect(process.Wait()).To(Equal(0))
			})

			It("does not leak open files", func() {
				openFileCount := func() int {
					procFd := fmt.Sprintf("/proc/%d/fd", client.Pid)
					files, err := ioutil.ReadDir(procFd)
					Expect(err).ToNot(HaveOccurred())
					return len(files)
				}

				initialOpenFileCount := openFileCount()

				for i := 0; i < 50; i++ {
					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "true",
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(process.Wait()).To(Equal(0))
				}

				// there's some noise in 'open files' check, but it shouldn't grow
				// linearly with the number of processes spawned
				Eventually(openFileCount, "10s").Should(BeNumerically("<", initialOpenFileCount+10))
			})

		})

		Context("after destroying the container", func() {
			It("should return api.ContainerNotFoundError when deleting the container again", func() {
				Expect(client.Destroy(container.Handle())).To(Succeed())
				Expect(client.Destroy(container.Handle())).To(MatchError(garden.ContainerNotFoundError{container.Handle()}))
				container = nil
			})

			It("should ensure any iptables rules which were created no longer exist", func() {
				handle := container.Handle()
				Expect(client.Destroy(handle)).To(Succeed())
				container = nil

				iptables, err := gexec.Start(exec.Command("iptables", "-L"), GinkgoWriter, GinkgoWriter)
				Expect(err).ToNot(HaveOccurred())
				Eventually(iptables, "10s").Should(gexec.Exit())
				Expect(iptables).ToNot(gbytes.Say(handle))
			})

			It("destroys multiple containers based on same rootfs", func() {
				c1, err := client.Create(garden.ContainerSpec{
					RootFSPath: "docker:///busybox",
					Privileged: false,
				})
				Expect(err).ToNot(HaveOccurred())
				c2, err := client.Create(garden.ContainerSpec{
					RootFSPath: "docker:///busybox",
					Privileged: false,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(client.Destroy(c1.Handle())).To(Succeed())
				Expect(client.Destroy(c2.Handle())).To(Succeed())
			})

			It("should not leak network namespace", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())
				Expect(info.State).To(Equal("active"))

				pidPath := filepath.Join(info.ContainerPath, "run", "wshd.pid")

				_, err = ioutil.ReadFile(pidPath)
				Expect(err).ToNot(HaveOccurred())

				Expect(client.Destroy(container.Handle())).To(Succeed())
				container = nil

				stdout := gbytes.NewBuffer()
				cmd, err := gexec.Start(
					exec.Command(
						"sh",
						"-c",
						"mount -n -t tmpfs tmpfs /sys && ip netns list && umount /sys",
					),
					stdout,
					GinkgoWriter,
				)

				Expect(err).ToNot(HaveOccurred())
				Expect(cmd.Wait("2s").ExitCode()).To(Equal(0))
				Expect(stdout.Contents()).To(Equal([]byte{}))
			})
		})
	})
})
