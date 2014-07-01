package lifecycle_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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
			processID, runStream, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			})

			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			Eventually(runStream).Should(BeClosed())

			stream, err := container.Attach(processID)
			Ω(err).ShouldNot(HaveOccurred())

			var chunk warden.ProcessStream
			Eventually(stream).Should(Receive(&chunk))
			Ω(chunk.Data).Should(ContainSubstring("hi\n"))
		})

		It("does not have its job ID repeated", func() {
			processID1, _, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			processID2, _, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(processID1).ShouldNot(Equal(processID2))
		})

		Context("that prints monotonously increasing output", func() {
			It("does not duplicate its output on reconnect", func(done Done) {
				receivedNumbers := make(chan int, 2048)

				processID, _, err := container.Run(warden.ProcessSpec{
					Path: "bash",
					Args: []string{"-c", "for i in $(seq 10); do echo $i; sleep 0.5; done; echo goodbye; while true; do sleep 1; done"},
				})
				Ω(err).ShouldNot(HaveOccurred())

				stream, err := container.Attach(processID)
				Ω(err).ShouldNot(HaveOccurred())

				go streamNumbersTo(receivedNumbers, stream)

				time.Sleep(500 * time.Millisecond)

				restartWarden()

				stream, err = container.Attach(processID)
				Ω(err).ShouldNot(HaveOccurred())

				go streamNumbersTo(receivedNumbers, stream)

				lastNum := 0
				for num := range receivedNumbers {
					Ω(num).Should(BeNumerically(">", lastNum))
					lastNum = num
				}

				close(done)
			}, 30.0)
		})
	})

	Describe("a memory limit", func() {
		It("is still enforced", func() {
			err := container.LimitMemory(warden.MemoryLimits{32 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			_, stream, err := container.Run(warden.ProcessSpec{
				Path: "ruby",
				Args: []string{"-e", "$stdout.sync = true; puts :hello; puts (\"x\" * 64 * 1024 * 1024).size; puts :goodbye; exit 42"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			// cgroups OOM killer seems to leave no trace of the process;
			// there's no exit status indicator, so just assert that the one
			// we tried to exit with after over-allocating is not seen

			stdout, _, exitStatus := readUntilExit(stream)
			Ω(stdout).Should(Equal("hello\n"))
			Ω(exitStatus).ShouldNot(Equal(uint32(42)))
		})
	})

	Describe("a container's active job", func() {
		It("is still tracked", func() {
			processID, _, err := container.Run(warden.ProcessSpec{
				Path: "bash",
				Args: []string{"-c", "while true; do echo hi; sleep 0.5; done"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			restartWarden()

			info, err := container.Info()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(info.ProcessIDs).Should(ContainElement(uint32(processID)))
		})
	})

	Describe("a container's list of events", func() {
		It("is still reported", func() {
			err := container.LimitMemory(warden.MemoryLimits{4 * 1024 * 1024})
			Ω(err).ShouldNot(HaveOccurred())

			// trigger 'out of memory' event
			_, stream, err := container.Run(warden.ProcessSpec{
				Path: "ruby",
				Args: []string{"-e", "$stdout.sync = true; puts :hello; puts (\"x\" * 5 * 1024 * 1024).size; puts :goodbye; exit 42"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			for _ = range stream {
				// wait until process exits
			}

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
			idA := ""
			idB := ""

			_, streamA, err := container.Run(warden.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			for chunk := range streamA {
				idA += string(chunk.Data)
			}

			restartWarden()

			otherContainer, err := client.Create(warden.ContainerSpec{})
			Ω(err).ShouldNot(HaveOccurred())

			_, streamB, err := otherContainer.Run(warden.ProcessSpec{
				Path: "id",
				Args: []string{"-u"},
			})
			Ω(err).ShouldNot(HaveOccurred())

			for chunk := range streamB {
				idB += string(chunk.Data)
			}

			Ω(idA).ShouldNot(Equal(idB))
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

func streamNumbersTo(destination chan<- int, source <-chan warden.ProcessStream) {
	for out := range source {
		buf := bytes.NewBuffer(out.Data)

		var num int

		for {
			_, err := fmt.Fscanf(buf, "%d\n", &num)
			if err == io.EOF {
				break
			}

			// got goodbye
			if err != nil {
				close(destination)
				return
			}

			destination <- num
		}
	}
}

func readUntilExit(stream <-chan warden.ProcessStream) (string, string, uint32) {
	stdout := ""
	stderr := ""
	exitStatus := uint32(12234)

	for payload := range stream {
		switch payload.Source {
		case warden.ProcessStreamSourceStdout:
			stdout += string(payload.Data)

		case warden.ProcessStreamSourceStderr:
			stderr += string(payload.Data)
		}

		if payload.ExitStatus != nil {
			exitStatus = *payload.ExitStatus
		}
	}

	return stdout, stderr, exitStatus
}
