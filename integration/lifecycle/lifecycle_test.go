package lifecycle_test

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
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
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	It("sources /etc/seed", func() {
		_, stream, err := container.Run(warden.ProcessSpec{
			Script: "test -e /tmp/ran-seed",
		})
		Expect(err).ToNot(HaveOccurred())

		for chunk := range stream {
			if chunk.ExitStatus != nil {
				Expect(*chunk.ExitStatus).To(Equal(uint32(0)))
			}
		}
	})

	It("should provide 64k of /dev/shm within the container", func() {
		command1 := "df|grep /dev/shm|grep 342678243768342867432"
		command2 := "mount|grep /dev/shm|grep tmpfs"
		_, _, err := container.Run(warden.ProcessSpec{
			Script: fmt.Sprintf("%s && %s", command1, command2),
		})
		Expect(err).ToNot(HaveOccurred())
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

	Context("and running a job", func() {
		It("sends output back in chunks until stopped", func() {
			_, stream, err := container.Run(warden.ProcessSpec{
				Script: "sleep 0.5; echo $FIRST; sleep 0.5; echo $SECOND; sleep 0.5; exit 42",
				EnvironmentVariables: []warden.EnvironmentVariable{
					warden.EnvironmentVariable{Key: "FIRST", Value: "hello"},
					warden.EnvironmentVariable{Key: "SECOND", Value: "goodbye"},
				},
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(string((<-stream).Data)).To(Equal("hello\n"))
			Expect(string((<-stream).Data)).To(Equal("goodbye\n"))
			Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))
		})

		Context("and then attaching to it", func() {
			It("sends output back in chunks until stopped", func(done Done) {
				processID, _, err := container.Run(warden.ProcessSpec{
					Script: "sleep 2; echo hello; sleep 0.5; echo goodbye; sleep 0.5; exit 42",
				})
				Expect(err).ToNot(HaveOccurred())

				stream, err := container.Attach(processID)

				Expect(string((<-stream).Data)).To(Equal("hello\n"))
				Expect(string((<-stream).Data)).To(Equal("goodbye\n"))
				Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))

				close(done)
			}, 10.0)
		})

		Context("and then sending a Stop request", func() {
			It("terminates all running processes", func() {
				_, stream, err := container.Run(warden.ProcessSpec{
					Script: `exec ruby -e 'trap("TERM") { exit 42 }; while true; sleep 1; end'`,
				})

				Expect(err).ToNot(HaveOccurred())

				err = container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))
			})

			It("recursively terminates all child processes", func(done Done) {
				defer close(done)

				_, stream, err := container.Run(warden.ProcessSpec{
					Script: `
# don't die until child processes die
trap wait SIGTERM

# spawn child that exits when it receives TERM
bash -c 'sleep 100 & wait' &

# wait on children
wait
`,
				})

				Expect(err).ToNot(HaveOccurred())

				stoppedAt := time.Now()

				err = container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

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
						Script: `exec ruby -e 'trap("TERM") { puts "cant touch this" }; sleep 1000'`,
					})

					Expect(err).ToNot(HaveOccurred())

					stoppedAt := time.Now()

					err = container.Stop(false)
					Expect(err).ToNot(HaveOccurred())

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
			tarInput, err := container.StreamIn("/tmp/some-container-dir")
			Ω(err).ShouldNot(HaveOccurred())

			_, err = io.Copy(tarInput, tarStream)
			Ω(err).ShouldNot(HaveOccurred())

			tarInput.Close()

			_, stream, err := container.Run(warden.ProcessSpec{
				Script: `test -f /tmp/some-container-dir/some-temp-dir/some-temp-file && exit 42`,
			})

			Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))
		})

		It("returns an error when the tar process dies", func(done Done) {
			tarInput, err := container.StreamIn("/tmp/some-container-dir")
			Ω(err).ShouldNot(HaveOccurred())

			err = container.Stop(true)
			Ω(err).ShouldNot(HaveOccurred())

			_, err = io.Copy(tarInput, tarStream)
			Ω(err).ShouldNot(HaveOccurred())

			err = tarInput.Close()
			Ω(err).Should(HaveOccurred())

			close(done)
		}, 3)

		Context("and then copying them out", func() {
			It("streams the directory", func() {
				_, stream, err := container.Run(warden.ProcessSpec{
					Script: `mkdir -p some-outer-dir/some-inner-dir; touch some-outer-dir/some-inner-dir/some-file;`,
				})

				Expect(*(<-stream).ExitStatus).To(Equal(uint32(0)))

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
						Script: `mkdir -p some-container-dir; touch some-container-dir/some-file;`,
					})

					Expect(*(<-stream).ExitStatus).To(Equal(uint32(0)))

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
			Expect(err).ToNot(HaveOccurred())

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("stopped"))
		})
	})
})
