package lifecycle_test

import (
	"fmt"
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
					User: "alice",
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
					User: "alice",
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
						User: "alice",
						Path: "true",
					}, garden.ProcessIO{})
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
