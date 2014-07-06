package lifecycle_test

import (
	"fmt"
	"io"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/garden/warden"
)

var _ = Describe("Through a restart", func() {
	var container warden.Container

	BeforeEach(func() {
		client = startWarden()

		var err error

		container, err = client.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("retains the container list", func() {
		restartWarden()

		handles := getContainerHandles()
		Ω(handles).Should(ContainElement(container.Handle()))
	})

	Describe("a started job", func() {
		It("continues to stream", func() {
			process, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			_, err = process.Wait()
			Ω(err).Should(HaveOccurred())

			stdout := gbytes.NewBuffer()
			_, err = container.Attach(process.ID(), warden.ProcessIO{
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("hi\n"))
		})

		It("does not have its job ID repeated", func() {
			process1, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			process2, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process1.ID()).ShouldNot(Equal(process2.ID()))
		})

		Context("that prints monotonously increasing output", func() {
			It("does not duplicate its output on reconnect", func() {
				receivedNumbers := make(chan int, 16)

				process, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", "for i in $(seq 10); do echo $i; sleep 0.5; done; echo -1; while true; do sleep 1; done"},
				}, warden.ProcessIO{})
				Ω(err).ShouldNot(HaveOccurred())

				stdoutR, stdoutW := io.Pipe()

				_, err = container.Attach(process.ID(), warden.ProcessIO{
					Stdout: stdoutW,
				})
				Ω(err).ShouldNot(HaveOccurred())

				firstStream := &sync.WaitGroup{}
				firstStream.Add(1)

				go func() {
					streamNumbersTo(receivedNumbers, stdoutR)
					firstStream.Done()
				}()

				time.Sleep(time.Second)

				restartWarden()

				stdoutW.Close()

				firstStream.Wait()

				stdoutR, stdoutW = io.Pipe()

				_, err = container.Attach(process.ID(), warden.ProcessIO{
					Stdout: stdoutW,
				})
				Ω(err).ShouldNot(HaveOccurred())

				go streamNumbersTo(receivedNumbers, stdoutR)

				lastNum := 0
				for num := range receivedNumbers {
					Ω(num).Should(BeNumerically(">", lastNum))
					lastNum = num
				}
			})
		})
	})

	Describe("a memory limit", func() {
		It("is still enforced", func() {
			err := container.LimitMemory(warden.MemoryLimits{32 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			process, err := container.Run(warden.ProcessSpec{
				Path: "ruby",
				Args: []string{"-e", "$stdout.sync = true; puts :hello; puts (\"x\" * 64 * 1024 * 1024).size; puts :goodbye; exit 42"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			// cgroups OOM killer seems to leave no trace of the process;
			// there's no exit status indicator, so just assert that the one
			// we tried to exit with after over-allocating is not seen

			Ω(process.Wait()).ShouldNot(Equal(42), "process did not get OOM killed")
		})
	})

	Describe("a container's active job", func() {
		It("is still tracked", func() {
			process, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.ProcessIDs).Should(ContainElement(uint32(process.ID())))
		})
	})

	Describe("a container's list of events", func() {
		It("is still reported", func() {
			err := container.LimitMemory(warden.MemoryLimits{4 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			// trigger 'out of memory' event
			process, err := container.Run(warden.ProcessSpec{
				Path: "ruby",
				Args: []string{"-e", "$stdout.sync = true; puts :hello; puts (\"x\" * 5 * 1024 * 1024).size; puts :goodbye; exit 42"},
			}, warden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).ShouldNot(Equal(42), "process did not get OOM killed")

			Eventually(func() []string {
				info, err := container.Info()
				Ω(err).ShouldNot(HaveOccurred())

				return info.Events
			}).Should(ContainElement("out of memory"))

			restartWarden()

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Events).Should(ContainElement("out of memory"))
		})
	})

	Describe("a container's properties", func() {
		It("are retained", func() {
			containerWithProperties, err := client.Create(warden.ContainerSpec{
				Properties: warden.Properties{
					"foo": "bar",
				},
			})
			Ω(err).ShouldNot(HaveOccurred())

			info, err := containerWithProperties.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.Properties["foo"]).Should(Equal("bar"))

			restartWarden()

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

			restartWarden()

			info, err = container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("active"))

			err = container.Stop(false)
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			info, err = container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.State).Should(Equal("stopped"))
		})
	})

	Describe("a container's network", func() {
		It("does not get reused", func() {
			infoA, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			newContainer, err := client.Create(warden.ContainerSpec{})
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

			restartWarden()

			containerB, err := client.Create(warden.ContainerSpec{})
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

			processA, err := container.Run(warden.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			}, warden.ProcessIO{
				Stdout: idA,
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(processA.Wait()).Should(Equal(0))

			restartWarden()

			otherContainer, err := client.Create(warden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			processB, err := otherContainer.Run(warden.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			}, warden.ProcessIO{Stdout: idB})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(processB.Wait()).Should(Equal(0))

			Ω(idA.Contents()).ShouldNot(Equal(idB.Contents()))
		})
	})

	Describe("a container's grace time", func() {
		BeforeEach(func() {
			restartWarden("--containerGraceTime", "5s")
		})

		It("is still enforced", func() {
			container, err := client.Create(warden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			Ω(getContainerHandles()).Should(ContainElement(container.Handle()))
			Eventually(getContainerHandles, 10*time.Second).ShouldNot(ContainElement(container.Handle()))
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

func streamNumbersTo(destination chan<- int, source io.Reader) {
	for {
		var num int

		_, err := fmt.Fscanf(source, "%d\n", &num)
		if err == io.EOF {
			break
		}

		// got end of stream
		if num == -1 {
			close(destination)
			return
		}

		destination <- num
	}
}
