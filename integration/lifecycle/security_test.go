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
				// List all files
				exec.Command(
					"lsof", "-a",
					// open by a specific process id (initd)
					"-p", strings.TrimSpace(string(ps.Out.Contents())),
					//	AND their FDs are not txt or mem
					"-d", "^txt,^mem",
					//	AND they are found in the host mount namespace
					"/",
				),
				GinkgoWriter, GinkgoWriter)

			Eventually(lsof.Wait()).Should(gexec.Exit())
			Expect(lsof.Out).To(gbytes.Say(`\A\z`))
		})
	})

	Describe("Mount namespace", func() {
		It("does not allow mounts in the container to show in the host", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{Privileged: true})
			Expect(err).ToNot(HaveOccurred())

			process, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "/bin/mkdir",
				Args: []string{"/home/alice/lawn"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			process, err = container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "/bin/mkdir",
				Args: []string{"/home/alice/gnome"},
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
				Args: []string{"--bind", "/home/alice/lawn", "/home/alice/gnome"},
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
	})

	Describe("Network namespace", func() {
		It("does not allow network configuration in the container to show in the host", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{Privileged: true})
			Expect(err).ToNot(HaveOccurred())

			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "ifconfig",
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
				Path: "ifconfig",
			}, garden.ProcessIO{
				Stdout: stdout,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err = process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(stdout).To(gbytes.Say(`lo:0`))

			cat := exec.Command("ifconfig")
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
			container, err := client.Create(garden.ContainerSpec{Privileged: true})
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

	Context("with an empty rootfs", func() {
		var (
			rootFSPath string
			container  garden.Container
		)

		BeforeEach(func() {
			rootFSPath = os.Getenv("GARDEN_EMPTY_TEST_ROOTFS")
			if rootFSPath == "" {
				Skip("GARDEN_EMPTY_TEST_ROOTFS undefined")
			}

			client = startGarden()
		})

		JustBeforeEach(func() {
			var err error

			container, err = client.Create(
				garden.ContainerSpec{
					RootFSPath: rootFSPath,
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("runs a statically compiled executable in the container", func() {
			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()
			process, err := container.Run(
				garden.ProcessSpec{
					User: "alice",
					Path: "/hello",
					Dir:  "/",
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

		Context("that has a list command", func() {
			BeforeEach(func() {
				var err error

				tempRootFSPath, err := ioutil.TempDir("", "")
				Expect(err).ToNot(HaveOccurred())
				cmd := exec.Command("bash", "-c", fmt.Sprintf("cp -R %s/* %s", rootFSPath, tempRootFSPath))
				Expect(cmd.Run()).To(Succeed())
				rootFSPath = tempRootFSPath

				lsPath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/integration/test-images/empty_ls")
				Expect(err).ToNot(HaveOccurred())
				cmd = exec.Command("cp", lsPath, path.Join(rootFSPath, "ls"))
				Expect(cmd.Run()).To(Succeed())
			})

			AfterEach(func() {
				os.RemoveAll(rootFSPath)
			})

			It("should only list known files and directories", func() {
				stdout := gbytes.NewBuffer()
				process, err := container.Run(
					garden.ProcessSpec{
						User: "alice",
						Path: "/ls",
						Dir:  "/",
					},
					garden.ProcessIO{
						Stdout: stdout,
						Stderr: GinkgoWriter,
					},
				)
				Expect(err).ToNot(HaveOccurred())

				exitStatus, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(exitStatus).To(Equal(0))
				Expect(string(stdout.Contents())).To(Equal(`dev
etc
hello
ls
proc
sys
tmp
`))
			})
		})
	})

	Describe("Denying access to network ranges", func() {
		var (
			blockedListener   garden.Container
			blockedListenerIP string = fmt.Sprintf("11.0.%d.2", GinkgoParallelNode())

			unblockedListener   garden.Container
			unblockedListenerIP string = fmt.Sprintf("11.1.%d.2", GinkgoParallelNode())

			allowedListener   garden.Container
			allowedListenerIP string = fmt.Sprintf("11.2.%d.2", GinkgoParallelNode())

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
				User: "alice",
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
