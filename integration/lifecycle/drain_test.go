package lifecycle_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/garden"
)

var _ = Describe("Through a restart", func() {
	var container garden.Container
	var gardenArgs []string
	var privileged bool

	BeforeEach(func() {
		gardenArgs = []string{}
		privileged = false
	})

	JustBeforeEach(func() {
		client = startGarden(gardenArgs...)

		var err error

		container, err = client.Create(garden.ContainerSpec{Privileged: privileged})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if container != nil {
			err := client.Destroy(container.Handle())
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("retains the container list", func() {
		restartGarden(gardenArgs...)

		handles := getContainerHandles()
		Expect(handles).To(ContainElement(container.Handle()))
	})

	It("allows us to run processes in the same container before and after restart", func() {
		By("running a process before restart")
		runEcho(container)

		restartGarden(gardenArgs...)

		By("and then running a process after restart")
		runEcho(container)
	})

	Describe("a started process", func() {
		It("continues to stream", func() {
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			_, err = process.Wait()
			Expect(err).To(HaveOccurred())

			stdout := gbytes.NewBuffer()
			_, err = container.Attach(process.ID(), garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout, 5.0).Should(gbytes.Say("hi\n"))
		})

		It("can still accept stdin", func() {
			r, w := io.Pipe()

			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "cat <&0"},
			}, garden.ProcessIO{
				Stdin:  r,
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = fmt.Fprintf(w, "hello")
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hello"))

			restartGarden(gardenArgs...)

			_, err = process.Wait()
			Expect(err).To(HaveOccurred())

			err = w.Close()
			Expect(err).ToNot(HaveOccurred())

			process, err = container.Attach(process.ID(), garden.ProcessIO{
				Stdin:  bytes.NewBufferString("world"),
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout, 10).Should(gbytes.Say("world"))
			Expect(process.Wait()).To(Equal(0))
		})

		It("can still have its tty window resized", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
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
				TTY: &garden.TTYSpec{
					WindowSize: &garden.WindowSize{
						Columns: 80,
						Rows:    24,
					},
				},
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("waiting"))

			restartGarden(gardenArgs...)

			_, err = process.Wait()
			Expect(err).To(HaveOccurred())

			inR, inW := io.Pipe()

			process, err = container.Attach(process.ID(), garden.ProcessIO{
				Stdin:  inR,
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

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

		It("does not have its job ID repeated", func() {
			process1, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			process2, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			Expect(process1.ID()).ToNot(Equal(process2.ID()))
		})

		It("can still be signalled", func() {
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", `
				  trap 'echo termed; exit 42' SIGTERM

					while true; do
					  echo waiting
					  sleep 1
					done
				`},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			stdout := gbytes.NewBuffer()
			attached, err := container.Attach(process.ID(), garden.ProcessIO{
				Stdout: io.MultiWriter(GinkgoWriter, stdout),
				Stderr: GinkgoWriter,
			})

			Eventually(stdout).Should(gbytes.Say("waiting"))
			Expect(attached.Signal(garden.SignalTerminate)).To(Succeed())
			Eventually(stdout, "2s").Should(gbytes.Say("termed"))
			Expect(attached.Wait()).To(Equal(42))
		})

		It("does not duplicate its output on reconnect", func() {
			stdinR, stdinW := io.Pipe()
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				Path: "cat",
			}, garden.ProcessIO{
				Stdin:  stdinR,
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			stdinW.Write([]byte("first-line\n"))
			Eventually(stdout).Should(gbytes.Say("first-line\n"))

			restartGarden(gardenArgs...)

			stdinR, stdinW = io.Pipe()
			stdout = gbytes.NewBuffer()

			_, err = container.Attach(process.ID(), garden.ProcessIO{
				Stdin:  stdinR,
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			stdinW.Write([]byte("second-line\n"))
			Eventually(stdout.Contents).Should(Equal([]byte("second-line\n")))
		})
	})

	Describe("a memory limit", func() {
		It("is still enforced", func() {
			err := container.LimitMemory(garden.MemoryLimits{4 * 1024 * 1024})
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $(yes | head -c 67108864); echo goodbye; exit 42"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			// cgroups OOM killer seems to leave no trace of the process;
			// there's no exit status indicator, so just assert that the one
			// we tried to exit with after over-allocating is not seen

			Expect(process.Wait()).ToNot(Equal(42), "process did not get OOM killed")
		})
	})

	Describe("a container's active job", func() {
		It("is still tracked", func() {
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.ProcessIDs).To(ContainElement(uint32(process.ID())))
		})
	})

	Describe("a container's list of events", func() {
		It("is still reported", func() {
			err := container.LimitMemory(garden.MemoryLimits{4 * 1024 * 1024})
			Expect(err).ToNot(HaveOccurred())

			// trigger 'out of memory' event
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo $(yes | head -c 67108864); echo goodbye; exit 42"},
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			Expect(process.Wait()).ToNot(Equal(42), "process did not get OOM killed")

			Eventually(func() []string {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				return info.Events
			}).Should(ContainElement("out of memory"))

			restartGarden(gardenArgs...)

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Events).To(ContainElement("out of memory"))
		})
	})

	Describe("a container's properties", func() {
		It("are retained", func() {
			containerWithProperties, err := client.Create(garden.ContainerSpec{
				Properties: garden.Properties{
					"foo": "bar",
				},
			})
			Expect(err).ToNot(HaveOccurred())

			info, err := containerWithProperties.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Properties["foo"]).To(Equal("bar"))

			restartGarden(gardenArgs...)

			info, err = containerWithProperties.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Properties["foo"]).To(Equal("bar"))
		})
	})

	Describe("a container's state", func() {
		It("is still reported", func() {
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("active"))

			restartGarden(gardenArgs...)

			info, err = container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("active"))

			err = container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			info, err = container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("stopped"))
		})
	})

	Describe("a container's network", func() {
		It("does not get reused", func() {
			infoA, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			newContainer, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			infoB, err := newContainer.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(infoA.HostIP).ToNot(Equal(infoB.HostIP))
			Expect(infoA.ContainerIP).ToNot(Equal(infoB.ContainerIP))
		})

		Context("when denying all networks initially", func() {
			var ByAllowingTCPTo func(net.IP)
			var ByDenyingTCPTo func(net.IP)
			var externalIP net.IP

			BeforeEach(func() {
				ips, err := net.LookupIP("www.example.com")
				Expect(err).ToNot(HaveOccurred())
				Expect(ips).ToNot(BeEmpty())
				externalIP = ips[0]

				gardenArgs = []string{
					"-denyNetworks", "0.0.0.0/0", // deny everything
					"-allowNetworks", "", // allow nothing
				}

				ByAllowingTCPTo = func(ip net.IP) {
					By("Allowing TCP to"+ip.String(), func() {
						process, _ := runInContainer(
							container,
							fmt.Sprintf("(echo 'GET / HTTP/1.1'; echo 'Host: example.com'; echo) | nc -w5 %s 80", ip),
						)
						status, err := process.Wait()
						Expect(err).ToNot(HaveOccurred())
						Expect(status).To(Equal(0))
					})
				}

				ByDenyingTCPTo = func(ip net.IP) {
					By("Denying TCP to"+ip.String(), func() {
						process, _ := runInContainer(
							container,
							fmt.Sprintf("(echo 'GET / HTTP/1.1'; echo 'Host: example.com'; echo) | nc -w5 %s 80", ip),
						)
						status, err := process.Wait()
						Expect(err).ToNot(HaveOccurred())
						Expect(status).ToNot(Equal(0))
					})
				}
			})

			It("preserves NetOut rules", func() {
				// Initially prevented from accessing (sanity check)
				ByDenyingTCPTo(externalIP)

				// Allow access
				Expect(container.NetOut(garden.NetOutRule{
					Protocol: garden.ProtocolTCP,
					Networks: []garden.IPRange{
						garden.IPRangeFromIP(externalIP),
					},
				})).To(Succeed())

				// Check it worked (sanity check)
				ByAllowingTCPTo(externalIP)

				restartGarden(gardenArgs...)
				ByAllowingTCPTo(externalIP)
			})
		})

	})

	Describe("a container's mapped port", func() {
		It("does not get reused", func() {
			netInAHost, netInAContainer, err := container.NetIn(0, 0)
			Expect(err).ToNot(HaveOccurred())

			restartGarden(gardenArgs...)

			containerB, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			netInBHost, netInBContainer, err := containerB.NetIn(0, 0)
			Expect(err).ToNot(HaveOccurred())

			Expect(netInAHost).ToNot(Equal(netInBHost))
			Expect(netInAContainer).ToNot(Equal(netInBContainer))
		})
	})

	Describe("a container's grace time", func() {
		BeforeEach(func() {
			gardenArgs = []string{"--containerGraceTime", "5s"}
		})

		It("is still enforced", func() {
			restartGarden(gardenArgs...)

			Expect(getContainerHandles()).To(ContainElement(container.Handle()))
			Eventually(getContainerHandles, 10*time.Second).ShouldNot(ContainElement(container.Handle()))
			container = nil
		})
	})

	Describe("a privileged container", func() {
		BeforeEach(func() {
			privileged = true
		})

		It("is still present", func() {
			restartGarden(gardenArgs...)
			Expect(getContainerHandles()).To(ContainElement(container.Handle()))
		})
	})
})

func getContainerHandles() []string {
	containers, err := client.Containers(nil)
	Expect(err).ToNot(HaveOccurred())

	handles := make([]string, len(containers))
	for i, c := range containers {
		handles[i] = c.Handle()
	}

	return handles
}

func runInContainer(container garden.Container, script string) (garden.Process, *gbytes.Buffer) {
	out := gbytes.NewBuffer()
	process, err := container.Run(garden.ProcessSpec{
		Path: "sh",
		Args: []string{"-c", script},
	}, garden.ProcessIO{
		Stdout: io.MultiWriter(out, GinkgoWriter),
		Stderr: GinkgoWriter,
	})
	Expect(err).ToNot(HaveOccurred())

	return process, out
}

func runEcho(container garden.Container) {
	process, _ := runInContainer(container, "echo hello")
	status, err := process.Wait()
	Expect(err).ToNot(HaveOccurred())
	Expect(status).To(Equal(0))
}
