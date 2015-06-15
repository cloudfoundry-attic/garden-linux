package lifecycle_test

import (
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden"

	"os/exec"

	"io"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Security", func() {
	Describe("PID namespace", func() {
		It("isolates processes so that only processes from inside the container are visible", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "sleep",
				Args: []string{"989898"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() []string {
				psout := gbytes.NewBuffer()
				ps, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "sh",
					Args: []string{"-c", "ps -a"},
				}, garden.ProcessIO{
					Stdout: psout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(ps.Wait()).To(Equal(0))
				return strings.Split(string(psout.Contents()), "\n")
			}).Should(HaveLen(6)) // header, wshd, sleep, sh, ps, \n
		})

		It("does not leak fds in to spawned processes", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "ls",
				Args: []string{"/proc/self/fd"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).To(gbytes.Say("0\n1\n2\n3\n")) // stdin, stdout, stderr, /proc/self/fd
		})

		It("does not keep any host files open", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			ps, err := gexec.Start(
				exec.Command("sh", "-c",
					fmt.Sprintf("ps -A -opid,args | grep wshd | grep %s | head -n 1 | awk '{ print $1 }'", container.Handle())),
				GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(ps).Should(gexec.Exit(0))

			lsof, err := gexec.Start(
				exec.Command("lsof", "-p", strings.TrimSpace(string(ps.Out.Contents()))),
				GinkgoWriter, GinkgoWriter)

			Eventually(lsof).Should(gexec.Exit(0))
			Expect(lsof).NotTo(gbytes.Say(container.Handle()))
		})

		It("has the correct initial process", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/ps",
				Args: []string{"-o", "pid,args"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).To(gbytes.Say(`\s+1\s+{initd}\s+wshd: %s`, container.Handle()))
		})
	})

	Describe("Mount namespace", func() {
		It("does not allow mounts in the container to show in the host", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			process, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/bin/mkdir",
				Args: []string{"/home/vcap/lawn"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			process, err = container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "/bin/mkdir",
				Args: []string{"/home/vcap/gnome"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			process, err = container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/mount",
				Args: []string{"--bind", "/home/vcap/lawn", "/home/vcap/gnome"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			stdout := gbytes.NewBuffer()
			process, err = container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/cat",
				Args: []string{"/proc/mounts"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).To(gbytes.Say(`gnome`))

			cat := exec.Command("/bin/cat", "/proc/mounts")
			catSession, err := gexec.Start(cat, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(catSession).Should(gexec.Exit(0))
			Expect(catSession).ToNot(gbytes.Say("gnome"))
		})

		It("unmounts /tmp/garden-host* in the container", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/cat",
				Args: []string{"/proc/mounts"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))
			Expect(stdout).ToNot(gbytes.Say(` /tmp/garden-host`))
		})
	})

	Describe("Network namespace", func() {
		It("does not allow network configuration in the container to show in the host", func() {

			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/sbin/ifconfig",
				Args: []string{"lo:0", "1.2.3.4", "up"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			stdout := gbytes.NewBuffer()
			process, err = container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/sbin/ifconfig",
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).To(gbytes.Say(`lo:0`))

			cat := exec.Command("/sbin/ifconfig")
			catSession, err := gexec.Start(cat, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(catSession).Should(gexec.Exit(0))
			Expect(catSession).ToNot(gbytes.Say("lo:0"))
		})
	})

	Describe("IPC namespace", func() {
		var sharedDir string
		var container garden.Container

		BeforeEach(func() {
			var err error
			sharedDir, err = ioutil.TempDir("", "shared-mount")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.MkdirAll(sharedDir, 0755)).To(Succeed())
		})

		AfterEach(func() {
			if container != nil {
				Expect(client.Destroy(container.Handle())).To(Succeed())
			}
			if sharedDir != "" {
				Expect(os.RemoveAll(sharedDir)).To(Succeed())
			}
		})

		It("does not allow shared memory segments in the host to be accessed by the container", func() {
			Expect(copyFile(shmTestBin, path.Join(sharedDir, "shm_test"))).To(Succeed())

			client = startGarden()
			var err error
			container, err = client.Create(garden.ContainerSpec{
				Privileged: true,
				BindMounts: []garden.BindMount{{
					SrcPath: sharedDir,
					DstPath: "/mnt/shared",
				}},
			})
			Expect(err).ToNot(HaveOccurred())

			// Create shared memory segment in the host.
			localSHM := exec.Command(shmTestBin)
			createLocal, err := gexec.Start(
				localSHM,
				GinkgoWriter,
				GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred())
			Eventually(createLocal).Should(gbytes.Say("ok"))

			// Create shared memory segment in the container.
			// If there is no IPC namespace, this will collide with the segment in the host and fail.
			stdout := gbytes.NewBuffer()
			_, err = container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/mnt/shared/shm_test",
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			Eventually(stdout).Should(gbytes.Say("ok"))

			localSHM.Process.Signal(syscall.SIGUSR2)

			Eventually(createLocal).Should(gexec.Exit(0))

		})
	})

	Describe("UTS namespace", func() {
		It("changing the container's hostname does not affect the host's hostname", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/hostname",
				Args: []string{"newhostname"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			stdout := gbytes.NewBuffer()
			process, err = container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/hostname",
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))
			Expect(stdout).To(gbytes.Say(`newhostname`))

			localHostname := exec.Command("hostname")
			localHostnameSession, err := gexec.Start(localHostname, GinkgoWriter, GinkgoWriter)
			Eventually(localHostnameSession).Should(gexec.Exit(0))
			Expect(localHostnameSession).ToNot(gbytes.Say("newhostname"))
		})
	})

	Describe("File system", func() {
		It("/tmp is world-writable in the container", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "ls",
				Args: []string{"-al", "/tmp"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))
			Expect(stdout).To(gbytes.Say(`drwxrwxrwt`))
		})
	})

	Describe("Control groups", func() {
		It("places the container in the required cgroup subsystems", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/sh",
				Args: []string{"-c", "cat /proc/$$/cgroup"},
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			op := stdout.Contents()
			Expect(op).To(MatchRegexp(`\bcpu\b`))
			Expect(op).To(MatchRegexp(`\bcpuacct\b`))
			Expect(op).To(MatchRegexp(`\bcpuset\b`))
			Expect(op).To(MatchRegexp(`\bdevices\b`))
			Expect(op).To(MatchRegexp(`\bmemory\b`))
		})
	})

	Describe("Users and groups", func() {
		Context("when running a command in a working dir", func() {
			It("executes with setuid and setgid", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Dir:  "/usr",
					Path: "pwd",
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("^/usr\n"))
			})
		})

		Context("when running a command as a non-root user", func() {
			It("executes with setuid and setgid", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "/bin/sh",
					Args: []string{"-c", "id -u; id -g"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("10001\n10001\n"))
			})

			It("sets $HOME, $USER, and $PATH", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("HOME=/home/vcap\nPATH=/usr/local/bin:/usr/bin:/bin\nPWD=/home/vcap\nSHLVL=1\nUSER=vcap\n"))
			})

			Context("when $HOME is set in the spec", func() {
				It("sets $HOME from the spec", func() {
					client = startGarden()
					container, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())

					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: "vcap",
						Path: "/bin/sh",
						Args: []string{"-c", "echo $HOME"},
						Env: []string{
							"HOME=/nowhere",
						},
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())

					exitStatus, err := process.Wait()
					Expect(err).ToNot(HaveOccurred())
					Expect(exitStatus).To(Equal(0))
					Expect(stdout).To(gbytes.Say("/nowhere"))
				})
			})

			It("executes in the user's home directory", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "/bin/pwd",
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("/home/vcap\n"))
			})

			It("sets the specified environment variables", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Env:  []string{"VAR1=VALUE1", "VAR2=VALUE2"},
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("VAR1=VALUE1\nVAR2=VALUE2\n"))
			})

			It("searches a sanitized path not including /sbin for the executable", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{
					GraceTime: time.Hour,
				})
				Expect(err).ToNot(HaveOccurred())

				process, err := container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "ls",
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())
				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))

				process, err = container.Run(garden.ProcessSpec{
					User: "vcap",
					Path: "ifconfig", // ifconfig is only available in /sbin
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())
				exitStatus, err = process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(255))
			})

		})

		Context("when running a command as root", func() {
			It("executes with setuid and setgid", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "id -u; id -g"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("0\n0\n"))
			})

			It("sets $HOME, $USER, and $PATH", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("HOME=/root\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\nPWD=/root\nSHLVL=1\nUSER=root\n"))
			})

			It("executes in root's home directory", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "/bin/pwd",
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("/root\n"))
			})

			It("sets the specified environment variables", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				stdout := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Env:  []string{"VAR1=VALUE1", "VAR2=VALUE2"},
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(stdout).To(gbytes.Say("VAR1=VALUE1\nVAR2=VALUE2\n"))
			})

			It("searches a sanitized path not including /sbin for the executable", func() {
				client = startGarden()
				container, err := client.Create(garden.ContainerSpec{
					GraceTime: time.Hour,
				})
				Expect(err).ToNot(HaveOccurred())

				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "ifconfig", // ifconfig is only available in /sbin
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())
				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
			})
		})
	})

	Context("with an empty rootfs", func() {
		var emptyRootFSPath string

		BeforeEach(func() {
			emptyRootFSPath = os.Getenv("GARDEN_EMPTY_TEST_ROOTFS")

			if emptyRootFSPath == "" {
				Fail("GARDEN_EMPTY_TEST_ROOTFS undefined;")
			}

			client = startGarden()
		})

		It("runs a statically compiled executable in the container", func() {
			container, err := client.Create(
				garden.ContainerSpec{
					RootFSPath: emptyRootFSPath,
				},
			)
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()
			process, err := container.Run(
				garden.ProcessSpec{
					User: "vcap",
					Path: "/hello",
				},
				garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				},
			)
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(string(stdout.Contents())).To(Equal("hello from stdout"))
			Expect(string(stderr.Contents())).To(Equal("hello from stderr"))
		})
	})

	Describe("Denying access to network ranges", func() {
		var (
			blockedListener   garden.Container
			blockedListenerIP string = fmt.Sprintf("11.0.%d.1", GinkgoParallelNode())

			unblockedListener   garden.Container
			unblockedListenerIP string = fmt.Sprintf("11.1.%d.1", GinkgoParallelNode())

			allowedListener   garden.Container
			allowedListenerIP string = fmt.Sprintf("11.2.%d.1", GinkgoParallelNode())

			sender garden.Container
		)

		BeforeEach(func() {
			client = startGarden(
				"-denyNetworks", strings.Join([]string{
					blockedListenerIP + "/32",
					allowedListenerIP + "/32",
				}, ","),
				"-allowNetworks", allowedListenerIP+"/32",
			)

			var err error

			// create a listener to which we deny network access
			blockedListener, err = client.Create(garden.ContainerSpec{Network: blockedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			blockedListenerIP = containerIP(blockedListener)

			// create a listener to which we do not deny access
			unblockedListener, err = client.Create(garden.ContainerSpec{Network: unblockedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			unblockedListenerIP = containerIP(unblockedListener)

			// create a listener to which we exclicitly allow access
			allowedListener, err = client.Create(garden.ContainerSpec{Network: allowedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			allowedListenerIP = containerIP(allowedListener)

			// create a container with the new deny network configuration
			sender, err = client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

		})

		AfterEach(func() {
			err := client.Destroy(sender.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(blockedListener.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(unblockedListener.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(allowedListener.Handle())
			Expect(err).ToNot(HaveOccurred())
		})

		runInContainer := func(container garden.Container, script string) garden.Process {
			process, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "sh",
				Args: []string{"-c", script},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			return process
		}

		It("makes that block of ip addresses inaccessible to the container", func() {
			runInContainer(blockedListener, "nc -l 0.0.0.0:12345")
			runInContainer(unblockedListener, "nc -l 0.0.0.0:12345")
			runInContainer(allowedListener, "nc -l 0.0.0.0:12345")

			// a bit of time for the listeners to start, since they block
			time.Sleep(time.Second)

			process := runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP),
			)
			Expect(process.Wait()).To(Equal(1))

			process = runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", unblockedListenerIP),
			)
			Expect(process.Wait()).To(Equal(0))

			process = runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", allowedListenerIP),
			)
			Expect(process.Wait()).To(Equal(0))
		})
	})
})

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}

	defer s.Close()

	d, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return err
	}

	_, err = io.Copy(d, s)
	if err != nil {
		d.Close()
		return err
	}

	return d.Close()
}
