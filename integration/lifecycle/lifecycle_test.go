package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	archiver "github.com/pivotal-golang/archiver/extractor/test_helper"
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

	Describe("Docker image download", func() {
		It("returns a helpful error message when image not found from default registry", func() {
			client = startGarden()
			_, err := client.Create(garden.ContainerSpec{RootFSPath: "docker:///cloudfoundry/doesnotexist"})
			Expect(err.Error()).To(ContainSubstring("could not fetch image cloudfoundry/doesnotexist from registry https://index.docker.io/v1/"))
		})

		It("returns a helpful error message when registry does not exist", func() {
			client = startGarden()

			// Note: Using a valid url that is not a docker registry would make the test assertion below fail due to a bug in
			//       docker https://github.com/docker/docker/blob/v1.3.3/registry/endpoint.go#L107-L157
			//       eg. client.Create(garden.ContainerSpec{RootFSPath: "docker://example.com/cloudfoundry/doesnotexist"})
			_, err := client.Create(garden.ContainerSpec{RootFSPath: "docker://does-not.exist/cloudfoundry/doesnotexist"})
			Expect(err.Error()).To(ContainSubstring("could not fetch image cloudfoundry/doesnotexist from registry does-not.exist"))
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

			Eventually(allBridges).ShouldNot(ContainSubstring(bridgePrefix))
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
			process, err := container.Run(garden.ProcessSpec{
				Path: "test",
				Args: []string{"-e", "/tmp/ran-seed"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))
		})

		It("provides /dev/shm as tmpfs in the container", func() {
			process, err := container.Run(garden.ProcessSpec{
				Path: "dd",
				Args: []string{"if=/dev/urandom", "of=/dev/shm/some-data", "count=64", "bs=1k"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))

			outBuf := gbytes.NewBuffer()

			process, err = container.Run(garden.ProcessSpec{
				Path: "cat",
				Args: []string{"/proc/mounts"},
			}, garden.ProcessIO{
				Stdout: outBuf,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).To(Equal(0))

			Expect(outBuf).To(gbytes.Say("tmpfs /dev/shm tmpfs"))
			Expect(outBuf).To(gbytes.Say("rw,nodev,relatime"))
		})

		Context("and sending a List request", func() {
			It("includes the created container", func() {
				Expect(getContainerHandles()).To(ContainElement(container.Handle()))
			})
		})

		Context("and sending an Info request", func() {
			It("returns the container's info", func() {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.State).To(Equal("active"))
			})
		})

		It("gives the container a hostname based on its id", func() {
			stdout := gbytes.NewBuffer()

			_, err := container.Run(garden.ProcessSpec{
				Path: "hostname",
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say(fmt.Sprintf("%s\n", container.Handle())))
		})

		Context("Using a docker image", func() {
			Context("when there is a VOLUME associated with the docker image", func() {
				BeforeEach(func() {
					// dockerfile contains `VOLUME /foo`, see diego-dockerfiles/with-volume
					rootfs = "docker:///cloudfoundry/with-volume"
				})

				It("creates the volume directory, if it does not already exist", func() {
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "ls",
						Args: []string{"-l", "/"},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})

					Expect(err).ToNot(HaveOccurred())

					process.Wait()
					Expect(stdout).To(gbytes.Say("foo"))
				})
			})

			Context("when the docker image specifies $PATH", func() {
				BeforeEach(func() {
					// Dockerfile contains:
					//   ENV PATH /usr/local/bin:/usr/bin:/bin:/from-dockerfile
					//   ENV TEST test-from-dockerfile
					//   ENV TEST second-test-from-dockerfile:$TEST
					// see diego-dockerfiles/with-volume
					rootfs = "docker:///cloudfoundry/with-volume"
				})

				It("$PATH is taken from the docker image", func() {
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "/bin/sh",
						Args: []string{"-c", "echo $PATH"},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})

					Expect(err).ToNot(HaveOccurred())

					process.Wait()
					Expect(stdout).To(gbytes.Say("/usr/local/bin:/usr/bin:/bin:/from-dockerfile"))
				})

				It("$TEST is taken from the docker image", func() {
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						Path: "/bin/sh",
						Args: []string{"-c", "echo $TEST"},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})

					Expect(err).ToNot(HaveOccurred())

					process.Wait()
					Expect(stdout).To(gbytes.Say("second-test-from-dockerfile:test-from-dockerfile"))
				})
			})
		})

		Context("and running a process", func() {
			It("runs as the vcap user by default", func() {
				stdout := gbytes.NewBuffer()

				_, err := container.Run(garden.ProcessSpec{
					Path: "whoami",
				}, garden.ProcessIO{
					Stdout: stdout,
				})

				Expect(err).ToNot(HaveOccurred())
				Eventually(stdout).Should(gbytes.Say("vcap\n"))
			})

			Context("when root is requested", func() {
				It("runs as root inside the container", func() {
					stdout := gbytes.NewBuffer()

					_, err := container.Run(garden.ProcessSpec{
						Path: "whoami",
						User: "root",
					}, garden.ProcessIO{
						Stdout: stdout,
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

				Context("by default", func() {
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
				})
			})

			Measure("it should stream stdout and stderr efficiently", func(b Benchmarker) {
				b.Time("(baseline) streaming 50M of stdout to /dev/null", func() {
					stdout := gbytes.NewBuffer()
					stderr := gbytes.NewBuffer()

					_, err := container.Run(garden.ProcessSpec{
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
						Path: "sh",
						Args: []string{"-c", "tr '\\0' 'a' < /dev/zero | dd count=50 bs=1M; echo done"},
					}, garden.ProcessIO{
						Stdout: stdout,
						Stderr: stderr,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout, "2s").Should(gbytes.Say("done\n"))
				})

				Expect(time.Seconds()).To(BeNumerically("<", 1))
			}, 10)

			It("streams output back and reports the exit status", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
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

			It("sends a KILL signal to the process if requested", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
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
				Expect(process.Wait()).ToNot(Equal(0))
			})

			It("avoids a race condition when sending a kill signal", func(done Done) {
				stdout := gbytes.NewBuffer()

				for i := 0; i < 200; i++ {
					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", `while true; do echo -n "x"; sleep 1; done`},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Signal(garden.SignalKill)).To(Succeed())
					Expect(process.Wait()).To(Equal(255))
				}
				close(done)
			}, 30.0)

			It("collects the process's full output, even if it exits quickly after", func() {
				for i := 0; i < 100; i++ {
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
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
					procFd := fmt.Sprintf("/proc/%d/fd", gardenRunner.Command.Process.Pid)
					files, err := ioutil.ReadDir(procFd)
					Expect(err).ToNot(HaveOccurred())
					return len(files)
				}

				initialOpenFileCount := openFileCount()

				for i := 0; i < 50; i++ {
					process, err := container.Run(garden.ProcessSpec{
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

			It("forwards the exit status even if stdin is still being written", func() {
				// this covers the case of intermediaries shuffling i/o around (e.g. wsh)
				// receiving SIGPIPE on write() due to the backing process exiting without
				// flushing stdin
				//
				// in practice it's flaky; sometimes write() finishes just before the
				// process exits, so run it ~10 times (observed it fail often in this range)

				for i := 0; i < 10; i++ {
					process, err := container.Run(garden.ProcessSpec{
						Path: "ls",
					}, garden.ProcessIO{
						Stdin: bytes.NewBufferString(strings.Repeat("x", 1024)),
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(0))
				}
			})

			Context("with a memory limit", func() {
				JustBeforeEach(func() {
					err := container.LimitMemory(garden.MemoryLimits{
						LimitInBytes: 64 * 1024 * 1024,
					})
					Expect(err).ToNot(HaveOccurred())
				})

				Context("when the process writes too much to /dev/shm", func() {
					It("is killed", func() {
						process, err := container.Run(garden.ProcessSpec{
							Path: "dd",
							Args: []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=65"},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).ToNot(Equal(0))
					})
				})
			})

			Context("with a tty", func() {
				It("executes the process with a raw tty with the given window size", func() {
					stdout := gbytes.NewBuffer()

					inR, inW := io.Pipe()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", "read foo; stty -a"},
						TTY: &garden.TTYSpec{
							WindowSize: &garden.WindowSize{
								Columns: 123,
								Rows:    456,
							},
						},
					}, garden.ProcessIO{
						Stdin:  inR,
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					_, err = inW.Write([]byte("hello"))
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("hello"))

					_, err = inW.Write([]byte("\n"))
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

					Expect(process.Wait()).To(Equal(0))
				})

				It("can have its terminal resized", func() {
					stdout := gbytes.NewBuffer()

					inR, inW := io.Pipe()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{
							"-c",
							`
							trap "stty -a" SIGWINCH

							# continuously read so that the trap can keep firing
							while true; do
								echo waiting
								if read; then
									exit 0
								fi
							done
						`,
						},
						TTY: &garden.TTYSpec{},
					}, garden.ProcessIO{
						Stdin:  inR,
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("waiting"))

					err = process.SetTTY(garden.TTYSpec{
						WindowSize: &garden.WindowSize{
							Columns: 123,
							Rows:    456,
						},
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

					_, err = fmt.Fprintf(inW, "ok\n")
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(0))
				})
			})

			Context("with a working directory", func() {
				It("executes with the working directory as the dir", func() {
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "pwd",
						Dir:  "/usr",
					}, garden.ProcessIO{
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("/usr\n"))
					Expect(process.Wait()).To(Equal(0))
				})
			})

			Context("with a working directory that does not exist", func() {
				It("changing to given directory fails, process returns with exit status 255 and provides proper error message in stderr", func() {
					stderr := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Dir:  "/asdf",
					}, garden.ProcessIO{
						Stderr: stderr,
					})
					Expect(err).ToNot(HaveOccurred())
					Eventually(stderr).Should(gbytes.Say("ERROR: failed to change directory to /asdf: No such file or directory"))
					Expect(process.Wait()).To(Equal(255))
				})
			})

			Context("and then attaching to it", func() {
				It("streams output and the exit status to the attached request", func(done Done) {
					stdout1 := gbytes.NewBuffer()
					stdout2 := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", "sleep 2; echo hello; sleep 0.5; echo goodbye; sleep 0.5; exit 42"},
					}, garden.ProcessIO{
						Stdout: stdout1,
					})
					Expect(err).ToNot(HaveOccurred())

					attached, err := container.Attach(process.ID(), garden.ProcessIO{
						Stdout: stdout2,
					})
					Expect(err).ToNot(HaveOccurred())

					time.Sleep(2 * time.Second)

					Eventually(stdout1).Should(gbytes.Say("hello\n"))
					Eventually(stdout1).Should(gbytes.Say("goodbye\n"))

					Eventually(stdout2).Should(gbytes.Say("hello\n"))
					Eventually(stdout2).Should(gbytes.Say("goodbye\n"))

					Expect(process.Wait()).To(Equal(42))
					Expect(attached.Wait()).To(Equal(42))

					close(done)
				}, 10.0)
			})

			Context("and then sending a Stop request", func() {
				It("terminates all running processes", func() {
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{
							"-c",
							`
						trap 'exit 42' SIGTERM

						# sync with test, and allow trap to fire when not sleeping
						while true; do
							echo waiting
							sleep 0.5
						done
						`,
						},
					}, garden.ProcessIO{
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout, 30).Should(gbytes.Say("waiting"))

					err = container.Stop(false)
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(42))
				})

				It("recursively terminates all child processes", func(done Done) {
					defer close(done)

					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{
							"-c",
							`
						# don't die until child processes die
						trap wait SIGTERM

						# spawn child that exits when it receives TERM
						sh -c 'sleep 100 & wait' &

						# sync with test
						echo waiting

						# wait on children
						wait
						`,
						},
					}, garden.ProcessIO{
						Stdout: stdout,
					})

					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout, 5).Should(gbytes.Say("waiting\n"))

					stoppedAt := time.Now()

					err = container.Stop(false)
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(143)) // 143 = 128 + SIGTERM

					Expect(time.Since(stoppedAt)).To(BeNumerically("<=", 5*time.Second))
				}, 15)

				Context("when a process does not die 10 seconds after receiving SIGTERM", func() {
					It("is forcibly killed", func(done Done) {
						defer close(done)

						process, err := container.Run(garden.ProcessSpec{
							Path: "sh",
							Args: []string{
								"-c",
								`
                trap "echo cant touch this; sleep 1000" SIGTERM

                echo waiting
                sleep 1000 &
                wait
              `,
							},
						}, garden.ProcessIO{})

						Expect(err).ToNot(HaveOccurred())

						stoppedAt := time.Now()

						err = container.Stop(false)
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).ToNot(Equal(0)) // either 137 or 255

						Expect(time.Since(stoppedAt)).To(BeNumerically(">=", 10*time.Second))
					}, 15)
				})
			})
		})

		Context("and streaming files in", func() {
			var tarStream io.Reader

			JustBeforeEach(func() {
				tmpdir, err := ioutil.TempDir("", "some-temp-dir-parent")
				Expect(err).ToNot(HaveOccurred())

				tgzPath := filepath.Join(tmpdir, "some.tgz")

				archiver.CreateTarGZArchive(
					tgzPath,
					[]archiver.ArchiveFile{
						{
							Name: "./some-temp-dir",
							Dir:  true,
						},
						{
							Name: "./some-temp-dir/some-temp-file",
							Body: "some-body",
						},
					},
				)

				tgz, err := os.Open(tgzPath)
				Expect(err).ToNot(HaveOccurred())

				tarStream, err = gzip.NewReader(tgz)
				Expect(err).ToNot(HaveOccurred())
			})

			It("creates the files in the container, as the vcap user", func() {
				err := container.StreamIn("/tmp/some/container/dir", tarStream)
				Expect(err).ToNot(HaveOccurred())

				process, err := container.Run(garden.ProcessSpec{
					Path: "test",
					Args: []string{"-f", "/tmp/some/container/dir/some-temp-dir/some-temp-file"},
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))

				output := gbytes.NewBuffer()
				process, err = container.Run(garden.ProcessSpec{
					Path: "ls",
					Args: []string{"-al", "/tmp/some/container/dir/some-temp-dir/some-temp-file"},
				}, garden.ProcessIO{
					Stdout: output,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))

				// output should look like -rwxrwxrwx 1 vcap vcap 9 Jan  1  1970 /tmp/some-container-dir/some-temp-dir/some-temp-file
				Expect(output).To(gbytes.Say("vcap"))
				Expect(output).To(gbytes.Say("vcap"))
			})

			Context("in a privileged container", func() {
				BeforeEach(func() {
					privilegedContainer = true
				})

				It("streams in relative to the default run directory", func() {
					err := container.StreamIn(".", tarStream)
					Expect(err).ToNot(HaveOccurred())

					process, err := container.Run(garden.ProcessSpec{
						Path: "test",
						Args: []string{"-f", "some-temp-dir/some-temp-file"},
					}, garden.ProcessIO{})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(0))
				})
			})

			It("streams in relative to the default run directory", func() {
				err := container.StreamIn(".", tarStream)
				Expect(err).ToNot(HaveOccurred())

				process, err := container.Run(garden.ProcessSpec{
					Path: "test",
					Args: []string{"-f", "some-temp-dir/some-temp-file"},
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))
			})

			It("returns an error when the tar process dies", func() {
				err := container.StreamIn("/tmp/some-container-dir", &io.LimitedReader{
					R: tarStream,
					N: 10,
				})
				Expect(err).To(HaveOccurred())
			})

			Context("and then copying them out", func() {
				It("streams the directory", func() {
					process, err := container.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", `mkdir -p some-outer-dir/some-inner-dir && touch some-outer-dir/some-inner-dir/some-file`},
					}, garden.ProcessIO{})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(0))

					tarOutput, err := container.StreamOut("some-outer-dir/some-inner-dir")
					Expect(err).ToNot(HaveOccurred())

					tarReader := tar.NewReader(tarOutput)

					header, err := tarReader.Next()
					Expect(err).ToNot(HaveOccurred())
					Expect(header.Name).To(Equal("some-inner-dir/"))

					header, err = tarReader.Next()
					Expect(err).ToNot(HaveOccurred())
					Expect(header.Name).To(Equal("some-inner-dir/some-file"))
				})

				Context("with a trailing slash", func() {
					It("streams the contents of the directory", func() {
						process, err := container.Run(garden.ProcessSpec{
							Path: "sh",
							Args: []string{"-c", `mkdir -p some-container-dir && touch some-container-dir/some-file`},
						}, garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())

						Expect(process.Wait()).To(Equal(0))

						tarOutput, err := container.StreamOut("some-container-dir/")
						Expect(err).ToNot(HaveOccurred())

						tarReader := tar.NewReader(tarOutput)

						header, err := tarReader.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(header.Name).To(Equal("./"))

						header, err = tarReader.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(header.Name).To(Equal("./some-file"))
					})
				})
			})
		})

		Context("and sending a Stop request", func() {
			It("changes the container's state to 'stopped'", func() {
				err := container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.State).To(Equal("stopped"))
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
				Eventually(iptables, "2s").Should(gexec.Exit())
				Expect(iptables).ToNot(gbytes.Say(handle))
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
				Expect(cmd.Wait("1s").ExitCode()).To(Equal(0))
				Expect(stdout.Contents()).To(Equal([]byte{}))
			})
		})
	})
})
