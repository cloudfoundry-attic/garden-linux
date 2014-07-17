package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
		client = startWarden()

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

	It("provides 64k of /dev/shm within the container", func() {
		process, err := container.Run(warden.ProcessSpec{
			Path: "dd",
			Args: []string{"if=/dev/urandom", "of=/dev/shm/just-enough", "count=64", "bs=1k"},
		}, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(0))

		process, err = container.Run(warden.ProcessSpec{
			Path: "dd",
			Args: []string{"if=/dev/urandom", "of=/dev/shm/just-over", "count=1", "bs=1k"},
		}, warden.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())

		Ω(process.Wait()).Should(Equal(1))
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
				Path: "bash",
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

		It("streams input to the process's stdin", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "cat <&0"},
			}, warden.ProcessIO{
				Stdin:  bytes.NewBufferString("hello\nworld"),
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hello\nworld"))
			Ω(process.Wait()).Should(Equal(0))
		})

		Context("with a tty", func() {
			It("executes the process with a raw tty", func() {
				stdout := gbytes.NewBuffer()

				inR, inW := io.Pipe()

				process, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", "read foo"},
					TTY:  true,
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

				Ω(process.Wait()).Should(Equal(0))
			})

			It("can have its terminal resized", func() {
				stdout := gbytes.NewBuffer()

				process, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{
						"-c",
						`
						trap 'stty -a; exit 42' WINCH

						while true; do
							echo waiting
							sleep 0.5
						done
						`,
					},
					TTY: true,
				}, warden.ProcessIO{
					Stdout: stdout,
				})
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("waiting"))

				err = process.SetWindowSize(123, 456)
				Ω(err).ShouldNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

				Ω(process.Wait()).Should(Equal(42))
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
					Path: "bash",
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
					Path: "bash",
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
					Path: "bash",
					Args: []string{
						"-c",
						`
						# don't die until child processes die
						trap wait SIGTERM

						# spawn child that exits when it receives TERM
						bash -c 'sleep 100 & wait' &

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
						Path: "ruby",
						Args: []string{"-e", `trap("TERM") { puts "cant touch this" }; sleep 1000`},
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

		It("creates the files in the container", func() {
			err := container.StreamIn("/tmp/some-container-dir", tarStream)
			Ω(err).ShouldNot(HaveOccurred())

			process, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", `test -f /tmp/some-container-dir/some-temp-dir/some-temp-file`},
			}, warden.ProcessIO{})

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
					Path: "bash",
					Args: []string{"-c", `mkdir -p some-outer-dir/some-inner-dir; touch some-outer-dir/some-inner-dir/some-file;`},
				}, warden.ProcessIO{})

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
						Path: "bash",
						Args: []string{"-c", `mkdir -p some-container-dir; touch some-container-dir/some-file;`},
					}, warden.ProcessIO{})

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
