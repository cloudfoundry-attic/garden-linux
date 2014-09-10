package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	archiver "github.com/pivotal-golang/archiver/extractor/test_helper"
)

var _ = Describe("Creating a container", func() {
	var container warden.Container

	BeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("sources /etc/seed", func() {
		process, err := container.Run(warden.ProcessSpec{
			Path: "test",
			Args: []string{"-e", "/tmp/ran-seed"},
		}, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(0))
	})

	It("provides /dev/shm as tmpfs in the container", func() {
		process, err := container.Run(warden.ProcessSpec{
			Path: "dd",
			Args: []string{"if=/dev/urandom", "of=/dev/shm/some-data", "count=64", "bs=1k"},
		}, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(0))

		outBuf := gbytes.NewBuffer()

		process, err = container.Run(warden.ProcessSpec{
			Path: "cat",
			Args: []string{"/proc/mounts"},
		}, warden.ProcessIO{
			Stdout: outBuf,
		})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(0))

		Ω(outBuf).Should(gbytes.Say("tmpfs /dev/shm tmpfs rw,relatime 0 0"))
	})

	Context("and sending a List request", func() {
		It("includes the created container", func() {
			Ω(getContainerHandles()).Should(ContainElement(container.Handle()))
		})
	})

	Context("and sending an Info request", func() {
		It("returns the container's info", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("active"))
		})
	})

	Context("and running a process", func() {
		It("streams output back and reports the exit status", func() {
			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()

			process, err := container.Run(warden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "sleep 0.5; echo $FIRST; sleep 0.5; echo $SECOND >&2; sleep 0.5; exit 42"},
				Env:  []string{"FIRST=hello", "SECOND=goodbye"},
			}, warden.ProcessIO{
				Stdout: stdout,
				Stderr: stderr,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hello\n"))
			Eventually(stderr).Should(gbytes.Say("goodbye\n"))
			Ω(process.Wait()).Should(Equal(42))
		})

		It("collects the process's full output, even if it exits quickly after", func() {
			for i := 0; i < 1000; i++ {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "cat <&0"},
				}, warden.ProcessIO{
					Stdin:  bytes.NewBuffer([]byte("hi stdout")),
					Stderr: os.Stderr,
					Stdout: stdout,
				})

				if err != nil {
					println("ERROR: " + err.Error())
					select {}
				}

				Ω(err).ShouldNot(HaveOccurred())
				Ω(process.Wait()).Should(Equal(0))

				Ω(stdout).Should(gbytes.Say("hi stdout"))
			}
		})

		It("streams input to the process's stdin", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(warden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "cat <&0"},
			}, warden.ProcessIO{
				Stdin:  bytes.NewBufferString("hello\nworld"),
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hello\nworld"))
			Ω(process.Wait()).Should(Equal(0))
		})

		It("does not leak open files", func() {
			openFileCount := func() int {
				procFd := fmt.Sprintf("/proc/%d/fd", wardenRunner.Command.Process.Pid)
				files, err := ioutil.ReadDir(procFd)
				Ω(err).ShouldNot(HaveOccurred())
				return len(files)
			}

			initialOpenFileCount := openFileCount()

			for i := 0; i < 50; i++ {
				process, err := container.Run(warden.ProcessSpec{
					Path: "true",
				}, warden.ProcessIO{})
				Ω(err).ShouldNot(HaveOccurred())
				Ω(process.Wait()).Should(Equal(0))
			}

			// there's some noise in 'open files' check, but it shouldn't grow
			// linearly with the number of processes spawned
			Ω(openFileCount()).Should(BeNumerically("<", initialOpenFileCount+10))
		})

		It("forwards the exit status even if stdin is still being written", func() {
			// this covers the case of intermediaries shuffling i/o around (e.g. wsh)
			// receiving SIGPIPE on write() due to the backing process exiting without
			// flushing stdin
			//
			// in practice it's flaky; sometimes write() finishes just before the
			// process exits, so run it ~10 times (observed it fail often in this range)

			for i := 0; i < 10; i++ {
				process, err := container.Run(warden.ProcessSpec{
					Path: "ls",
				}, warden.ProcessIO{
					Stdin: bytes.NewBufferString(strings.Repeat("x", 1024)),
				})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.Wait()).Should(Equal(0))
			}
		})

		Context("with a memory limit", func() {
			BeforeEach(func() {
				err := container.LimitMemory(warden.MemoryLimits{
					LimitInBytes: 64 * 1024 * 1024,
				})
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when the process writes too much to /dev/shm", func() {
				It("is killed", func() {
					process, err := container.Run(warden.ProcessSpec{
						Path: "dd",
						Args: []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=65"},
					}, warden.ProcessIO{})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(process.Wait()).ShouldNot(Equal(0))
				})
			})
		})

		Context("with a tty", func() {
			It("executes the process with a raw tty with the given window size", func() {
				stdout := gbytes.NewBuffer()

				inR, inW := io.Pipe()

				process, err := container.Run(warden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "read foo; stty -a"},
					TTY: &warden.TTYSpec{
						WindowSize: &warden.WindowSize{
							Columns: 123,
							Rows:    456,
						},
					},
				}, warden.ProcessIO{
					Stdin:  inR,
					Stdout: stdout,
				})
				Ω(err).ShouldNot(HaveOccurred())

				_, err = inW.Write([]byte("hello"))
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("hello"))

				_, err = inW.Write([]byte("\n"))
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

				Ω(process.Wait()).Should(Equal(0))
			})

			It("can have its terminal resized", func() {
				stdout := gbytes.NewBuffer()

				inR, inW := io.Pipe()

				process, err := container.Run(warden.ProcessSpec{
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
					TTY: &warden.TTYSpec{},
				}, warden.ProcessIO{
					Stdin:  inR,
					Stdout: stdout,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("waiting"))

				err = process.SetTTY(warden.TTYSpec{
					WindowSize: &warden.WindowSize{
						Columns: 123,
						Rows:    456,
					},
				})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

				_, err = fmt.Fprintf(inW, "ok\n")
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.Wait()).Should(Equal(0))
			})
		})

		Context("with a working directory", func() {
			It("executes with the working directory as the dir", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
					Path: "pwd",
					Dir:  "/usr",
				}, warden.ProcessIO{
					Stdout: stdout,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("/usr\n"))
				Ω(process.Wait()).Should(Equal(0))
			})
		})

		Context("and then attaching to it", func() {
			It("streams output and the exit status to the attached request", func(done Done) {
				stdout1 := gbytes.NewBuffer()
				stdout2 := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "sleep 2; echo hello; sleep 0.5; echo goodbye; sleep 0.5; exit 42"},
				}, warden.ProcessIO{
					Stdout: stdout1,
				})
				Ω(err).ShouldNot(HaveOccurred())

				attached, err := container.Attach(process.ID(), warden.ProcessIO{
					Stdout: stdout2,
				})
				Ω(err).ShouldNot(HaveOccurred())

				time.Sleep(2 * time.Second)

				Eventually(stdout1).Should(gbytes.Say("hello\n"))
				Eventually(stdout1).Should(gbytes.Say("goodbye\n"))

				Eventually(stdout2).Should(gbytes.Say("hello\n"))
				Eventually(stdout2).Should(gbytes.Say("goodbye\n"))

				Ω(process.Wait()).Should(Equal(42))
				Ω(attached.Wait()).Should(Equal(42))

				close(done)
			}, 10.0)
		})

		Context("and then sending a Stop request", func() {
			It("terminates all running processes", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
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
				}, warden.ProcessIO{
					Stdout: stdout,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout, 30).Should(gbytes.Say("waiting"))

				err = container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.Wait()).Should(Equal(42))
			})

			It("recursively terminates all child processes", func(done Done) {
				defer close(done)

				stdout := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
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
				}, warden.ProcessIO{
					Stdout: stdout,
				})

				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout, 5).Should(gbytes.Say("waiting\n"))

				stoppedAt := time.Now()

				err = container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.Wait()).Should(Equal(143)) // 143 = 128 + SIGTERM

				Ω(time.Since(stoppedAt)).Should(BeNumerically("<=", 5*time.Second))
			}, 15)

			Context("when a process does not die 10 seconds after receiving SIGTERM", func() {
				It("is forcibly killed", func(done Done) {
					defer close(done)

					process, err := container.Run(warden.ProcessSpec{
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
					}, warden.ProcessIO{})

					Ω(err).ShouldNot(HaveOccurred())

					stoppedAt := time.Now()

					err = container.Stop(false)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(process.Wait()).ShouldNot(Equal(0)) // either 137 or 255

					Ω(time.Since(stoppedAt)).Should(BeNumerically(">=", 10*time.Second))
				}, 15)
			})
		})
	})

	Context("and streaming files in", func() {
		var tarStream io.Reader

		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir("", "some-temp-dir-parent")
			Ω(err).ShouldNot(HaveOccurred())

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
			Ω(err).ShouldNot(HaveOccurred())

			tarStream, err = gzip.NewReader(tgz)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("creates the files in the container, as the vcap user", func() {
			err := container.StreamIn("/tmp/some/container/dir", tarStream)
			Ω(err).ShouldNot(HaveOccurred())

			process, err := container.Run(warden.ProcessSpec{
				Path: "test",
				Args: []string{"-f", "/tmp/some/container/dir/some-temp-dir/some-temp-file"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0))

			output := gbytes.NewBuffer()
			process, err = container.Run(warden.ProcessSpec{
				Path: "ls",
				Args: []string{"-al", "/tmp/some/container/dir/some-temp-dir/some-temp-file"},
			}, warden.ProcessIO{
				Stdout: output,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0))

			// output should look like -rwxrwxrwx 1 vcap vcap 9 Jan  1  1970 /tmp/some-container-dir/some-temp-dir/some-temp-file
			Ω(output).Should(gbytes.Say("vcap"))
			Ω(output).Should(gbytes.Say("vcap"))
		})

		It("streams in relative to the default run directory", func() {
			err := container.StreamIn(".", tarStream)
			Ω(err).ShouldNot(HaveOccurred())

			process, err := container.Run(warden.ProcessSpec{
				Path: "test",
				Args: []string{"-f", "some-temp-dir/some-temp-file"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0))
		})

		It("returns an error when the tar process dies", func() {
			err := container.StreamIn("/tmp/some-container-dir", &io.LimitedReader{
				R: tarStream,
				N: 10,
			})
			Ω(err).Should(HaveOccurred())
		})

		Context("and then copying them out", func() {
			It("streams the directory", func() {
				process, err := container.Run(warden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", `mkdir -p some-outer-dir/some-inner-dir && touch some-outer-dir/some-inner-dir/some-file`},
				}, warden.ProcessIO{})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(process.Wait()).Should(Equal(0))

				tarOutput, err := container.StreamOut("some-outer-dir/some-inner-dir")
				Ω(err).ShouldNot(HaveOccurred())

				tarReader := tar.NewReader(tarOutput)

				header, err := tarReader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(header.Name).Should(Equal("some-inner-dir/"))

				header, err = tarReader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(header.Name).Should(Equal("some-inner-dir/some-file"))
			})

			Context("with a trailing slash", func() {
				It("streams the contents of the directory", func() {
					process, err := container.Run(warden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", `mkdir -p some-container-dir && touch some-container-dir/some-file`},
					}, warden.ProcessIO{})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(process.Wait()).Should(Equal(0))

					tarOutput, err := container.StreamOut("some-container-dir/")
					Ω(err).ShouldNot(HaveOccurred())

					tarReader := tar.NewReader(tarOutput)

					header, err := tarReader.Next()
					Ω(err).ShouldNot(HaveOccurred())
					Ω(header.Name).Should(Equal("./"))

					header, err = tarReader.Next()
					Ω(err).ShouldNot(HaveOccurred())
					Ω(header.Name).Should(Equal("./some-file"))
				})
			})
		})
	})

	Context("and sending a Stop request", func() {
		It("changes the container's state to 'stopped'", func() {
			err := container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("stopped"))
		})
	})
})
