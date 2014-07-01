package lifecycle_test

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		_, stream, err := container.Run(warden.ProcessSpec{
			Path: "test",
			Args: []string{"-e", "/tmp/ran-seed"},
		})
		Ω(err).ShouldNot(HaveOccurred())

		for chunk := range stream {
			if chunk.ExitStatus != nil {
				Ω(*chunk.ExitStatus).Should(Equal(uint32(0)))
			}
		}
	})

	It("should provide 64k of /dev/shm within the container", func() {
		_, _, err := container.Run(warden.ProcessSpec{
			Path: "bash",
			Args: []string{"-c", `
				df|grep /dev/shm|grep 342678243768342867432 &&
				mount|grep /dev/shm|grep tmpfs`,
			},
		})
		Ω(err).ShouldNot(HaveOccurred())
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
		It("sends output back in chunks until stopped", func() {
			_, stream, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "sleep 0.5; echo $FIRST; sleep 0.5; echo $SECOND; sleep 0.5; exit 42"},
				EnvironmentVariables: []warden.EnvironmentVariable{
					warden.EnvironmentVariable{Key: "FIRST", Value: "hello"},
					warden.EnvironmentVariable{Key: "SECOND", Value: "goodbye"},
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(string((<-stream).Data)).Should(Equal("hello\n"))
			Ω(string((<-stream).Data)).Should(Equal("goodbye\n"))
			Ω(*(<-stream).ExitStatus).Should(Equal(uint32(42)))
		})

		Context("and then attaching to it", func() {
			It("sends output back in chunks until stopped", func(done Done) {
				processID, _, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", "sleep 2; echo hello; sleep 0.5; echo goodbye; sleep 0.5; exit 42"},
				})
				Ω(err).ShouldNot(HaveOccurred())

				stream, err := container.Attach(processID)

				Ω(string((<-stream).Data)).Should(Equal("hello\n"))
				Ω(string((<-stream).Data)).Should(Equal("goodbye\n"))
				Ω(*(<-stream).ExitStatus).Should(Equal(uint32(42)))

				close(done)
			}, 10.0)
		})

		Context("and then sending a Stop request", func() {
			It("terminates all running processes", func() {
				_, stream, err := container.Run(warden.ProcessSpec{
					Path: "ruby",
					Args: []string{"-e", `trap("TERM") { exit 42 }; while true; sleep 1; end`},
				})

				Ω(err).ShouldNot(HaveOccurred())

				err = container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(*(<-stream).ExitStatus).Should(Equal(uint32(42)))
			})

			It("recursively terminates all child processes", func(done Done) {
				defer close(done)

				_, stream, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", `
# don't die until child processes die
trap wait SIGTERM

# spawn child that exits when it receives TERM
bash -c 'sleep 100 & wait' &

# wait on children
wait
`,
					},
				})

				Ω(err).ShouldNot(HaveOccurred())

				stoppedAt := time.Now()

				err = container.Stop(false)
				Ω(err).ShouldNot(HaveOccurred())

				for chunk := range stream {
					if chunk.ExitStatus != nil {
						// should have sigtermmed
						Ω(*chunk.ExitStatus).Should(Equal(uint32(143)))
					}
				}

				Ω(time.Since(stoppedAt)).Should(BeNumerically("<=", 5*time.Second))
			}, 15.0)

			Context("when a process does not die 10 seconds after receiving SIGTERM", func() {
				It("is forcibly killed", func() {
					_, stream, err := container.Run(warden.ProcessSpec{
						Path: "ruby",
						Args: []string{"-e", `trap("TERM") { puts "cant touch this" }; sleep 1000`},
					})

					Ω(err).ShouldNot(HaveOccurred())

					stoppedAt := time.Now()

					err = container.Stop(false)
					Ω(err).ShouldNot(HaveOccurred())

					Eventually(func() *uint32 {
						select {
						case chunk := <-stream:
							return chunk.ExitStatus
						default:
						}

						return nil
					}, 11.0).ShouldNot(BeNil())

					Ω(time.Since(stoppedAt)).Should(BeNumerically(">=", 10*time.Second))
				})
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

			_, stream, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", `test -f /tmp/some-container-dir/some-temp-dir/some-temp-file && exit 42`},
			})

			Ω(*(<-stream).ExitStatus).Should(Equal(uint32(42)))
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
				_, stream, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", `mkdir -p some-outer-dir/some-inner-dir; touch some-outer-dir/some-inner-dir/some-file;`},
				})

				Ω(*(<-stream).ExitStatus).Should(Equal(uint32(0)))

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
					_, stream, err := container.Run(warden.ProcessSpec{
						Path: "bash",
						Args: []string{"-c", `mkdir -p some-container-dir; touch some-container-dir/some-file;`},
					})

					Ω(*(<-stream).ExitStatus).Should(Equal(uint32(0)))

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
