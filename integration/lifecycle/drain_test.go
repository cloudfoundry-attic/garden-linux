package lifecycle_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/garden/api"
)

var _ = Describe("Through a restart", func() {
	var container api.Container
	var gardenArgs []string

	BeforeEach(func() {
		gardenArgs = []string{}
	})

	JustBeforeEach(func() {
		client = startGarden(gardenArgs...)

		var err error

		container, err = client.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			err := client.Destroy(container.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	It("retains the container list", func() {
		restartGarden()

		handles := getContainerHandles()
		Ω(handles).Should(ContainElement(container.Handle()))
	})

	Describe("a started process", func() {
		It("continues to stream", func() {
			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			_, err = process.Wait()
			Ω(err).Should(HaveOccurred())

			stdout := gbytes.NewBuffer()
			_, err = container.Attach(process.ID(), api.ProcessIO{
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout, 5.0).Should(gbytes.Say("hi\n"))
		})

		It("can still accept stdin", func() {
			r, w := io.Pipe()

			stdout := gbytes.NewBuffer()

			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "cat <&0"},
			}, api.ProcessIO{
				Stdin:  r,
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			_, err = fmt.Fprintf(w, "hello")
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hello"))

			restartGarden()

			_, err = process.Wait()
			Ω(err).Should(HaveOccurred())

			err = w.Close()
			Ω(err).ShouldNot(HaveOccurred())

			process, err = container.Attach(process.ID(), api.ProcessIO{
				Stdin:  bytes.NewBufferString("world"),
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout, 10).Should(gbytes.Say("world"))
			Ω(process.Wait()).Should(Equal(0))
		})

		It("can still have its tty window resized", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{
					"-c",

					// apparently, processes may receive SIGWINCH immediately upon
					// spawning. the initial approach was to exit after receiving the
					// signal, but sometimes it would exit immediately.
					//
					// so, instead, print whenever we receive SIGWINCH, and only exit
					// when a line of text is entered.
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
				TTY: &api.TTYSpec{
					WindowSize: &api.WindowSize{
						Columns: 80,
						Rows:    24,
					},
				},
			}, api.ProcessIO{
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("waiting"))

			restartGarden()

			_, err = process.Wait()
			Ω(err).Should(HaveOccurred())

			inR, inW := io.Pipe()

			process, err = container.Attach(process.ID(), api.ProcessIO{
				Stdin:  inR,
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			err = process.SetTTY(api.TTYSpec{
				WindowSize: &api.WindowSize{
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

		It("does not have its job ID repeated", func() {
			process1, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			process2, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process1.ID()).ShouldNot(Equal(process2.ID()))
		})

		It("can still be signalled", func() {
			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", `
				  trap 'echo termed; exit 42' SIGTERM

					while true; do
					  echo waiting
					  sleep 1
					done
				`},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			stdout := gbytes.NewBuffer()
			attached, err := container.Attach(process.ID(), api.ProcessIO{
				Stdout: io.MultiWriter(GinkgoWriter, stdout),
				Stderr: GinkgoWriter,
			})

			Eventually(stdout).Should(gbytes.Say("waiting"))
			Ω(attached.Signal(api.SignalTerminate)).Should(Succeed())
			Eventually(stdout, "2s").Should(gbytes.Say("termed"))
			Ω(attached.Wait()).Should(Equal(42))
		})

		It("does not duplicate its output on reconnect", func() {
			stdinR, stdinW := io.Pipe()
			stdout := gbytes.NewBuffer()

			process, err := container.Run(api.ProcessSpec{
				Path: "cat",
			}, api.ProcessIO{
				Stdin:  stdinR,
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			stdinW.Write([]byte("first-line\n"))
			Eventually(stdout).Should(gbytes.Say("first-line\n"))

			restartGarden()

			stdinR, stdinW = io.Pipe()
			stdout = gbytes.NewBuffer()

			_, err = container.Attach(process.ID(), api.ProcessIO{
				Stdin:  stdinR,
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			stdinW.Write([]byte("second-line\n"))
			Eventually(stdout.Contents).Should(Equal([]byte("second-line\n")))
		})
	})

	Describe("a memory limit", func() {
		It("is still enforced", func() {
			err := container.LimitMemory(api.MemoryLimits{4 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $(yes | head -c 67108864); echo goodbye; exit 42"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			// cgroups OOM killer seems to leave no trace of the process;
			// there's no exit status indicator, so just assert that the one
			// we tried to exit with after over-allocating is not seen

			Ω(process.Wait()).ShouldNot(Equal(42), "process did not get OOM killed")
		})
	})

	Describe("a container's active job", func() {
		It("is still tracked", func() {
			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.ProcessIDs).Should(ContainElement(uint32(process.ID())))
		})
	})

	Describe("a container's list of events", func() {
		It("is still reported", func() {
			err := container.LimitMemory(api.MemoryLimits{4 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			// trigger 'out of memory' event
			process, err := container.Run(api.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $(yes | head -c 67108864); echo goodbye; exit 42"},
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).ShouldNot(Equal(42), "process did not get OOM killed")

			Eventually(func() []string {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())

				return info.Events
			}).Should(ContainElement("out of memory"))

			restartGarden()

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Events).Should(ContainElement("out of memory"))
		})
	})

	Describe("a container's properties", func() {
		It("are retained", func() {
			containerWithProperties, err := client.Create(api.ContainerSpec{
				Properties: api.Properties{
					"foo": "bar",
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			info, err := containerWithProperties.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties["foo"]).Should(Equal("bar"))

			restartGarden()

			info, err = containerWithProperties.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties["foo"]).Should(Equal("bar"))
		})
	})

	Describe("a container's state", func() {
		It("is still reported", func() {
			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("active"))

			restartGarden()

			info, err = container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("active"))

			err = container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			info, err = container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("stopped"))
		})
	})

	Describe("a container's network", func() {
		It("does not get reused", func() {
			infoA, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			newContainer, err := client.Create(api.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			infoB, err := newContainer.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(infoA.HostIP).ShouldNot(Equal(infoB.HostIP))
			Ω(infoA.ContainerIP).ShouldNot(Equal(infoB.ContainerIP))
		})
	})

	Describe("a container's mapped port", func() {
		It("does not get reused", func() {
			netInAHost, netInAContainer, err := container.NetIn(0, 0)
			Ω(err).ShouldNot(HaveOccurred())

			restartGarden()

			containerB, err := client.Create(api.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			netInBHost, netInBContainer, err := containerB.NetIn(0, 0)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(netInAHost).ShouldNot(Equal(netInBHost))
			Ω(netInAContainer).ShouldNot(Equal(netInBContainer))
		})
	})

	Describe("a container's user", func() {
		It("does not get reused", func() {
			idA := gbytes.NewBuffer()
			idB := gbytes.NewBuffer()

			processA, err := container.Run(api.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			}, api.ProcessIO{
				Stdout: idA,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(processA.Wait()).Should(Equal(0))

			restartGarden()

			otherContainer, err := client.Create(api.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			processB, err := otherContainer.Run(api.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			}, api.ProcessIO{Stdout: idB})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(processB.Wait()).Should(Equal(0))

			Ω(idA.Contents()).ShouldNot(Equal(idB.Contents()))
		})
	})

	Describe("a container's grace time", func() {
		BeforeEach(func() {
			gardenArgs = []string{"--containerGraceTime", "5s"}
		})

		It("is still enforced", func() {
			restartGarden()

			Ω(getContainerHandles()).Should(ContainElement(container.Handle()))
			Eventually(getContainerHandles, 10*time.Second).ShouldNot(ContainElement(container.Handle()))
			container = nil
		})
	})
})

func getContainerHandles() []string {
	containers, err := client.Containers(nil)
	Ω(err).ShouldNot(HaveOccurred())

	handles := make([]string, len(containers))
	for i, c := range containers {
		handles[i] = c.Handle()
	}

	return handles
}
