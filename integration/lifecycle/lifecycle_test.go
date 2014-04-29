package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Creating a container", func() {
	var container warden.Container

	BeforeEach(func() {
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

	Context("and copying files in", func() {
		var path string

		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir("", "some-temp-dir-parent")
			Ω(err).ShouldNot(HaveOccurred())

			path = filepath.Join(tmpdir, "some-temp-dir")

			err = os.MkdirAll(path, 0755)
			Ω(err).ShouldNot(HaveOccurred())

			err = ioutil.WriteFile(filepath.Join(path, "some-temp-file"), []byte("HGJMT<"), 0755)
			Ω(err).ShouldNot(HaveOccurred())

		})

		It("creates the files in the container", func() {
			err := container.CopyIn(path, "/tmp/some-container-dir")
			Ω(err).ShouldNot(HaveOccurred())

			_, stream, err := container.Run(warden.ProcessSpec{
				Script: `test -f /tmp/some-container-dir/some-temp-dir/some-temp-file && exit 42`,
			})

			Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))
		})

		Context("with a strailing slash on the destination", func() {
			It("does what rsync does (syncs contents)", func() {
				err := container.CopyIn(path+"/", "/tmp/some-container-dir/")
				Ω(err).ShouldNot(HaveOccurred())

				_, stream, err := container.Run(warden.ProcessSpec{
					Script: `test -f /tmp/some-container-dir/some-temp-file && exit 42`,
				})

				Expect(*(<-stream).ExitStatus).To(Equal(uint32(42)))
			})
		})

		Context("and then copying them out", func() {
			It("copies the files to the host", func() {
				_, stream, err := container.Run(warden.ProcessSpec{
					Script: `mkdir -p some-container-dir; touch some-container-dir/some-file;`,
				})

				Expect(*(<-stream).ExitStatus).To(Equal(uint32(0)))

				tmpdir, err := ioutil.TempDir("", "copy-out-temp-dir-parent")
				Ω(err).ShouldNot(HaveOccurred())

				user, err := user.Current()
				Ω(err).ShouldNot(HaveOccurred())

				err = container.CopyOut("some-container-dir", tmpdir, user.Username)
				Ω(err).ShouldNot(HaveOccurred())

				_, err = os.Stat(filepath.Join(tmpdir, "some-container-dir", "some-file"))
				Ω(err).ShouldNot(HaveOccurred())
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
