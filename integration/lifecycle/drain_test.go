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

var _ = Describe("Through a restart", func() {
	var container warden.Container

	BeforeEach(func() {
		var err error

		container, err = client.Create(warden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
	})

	restartServer := func() {
		err := runner.Stop()
		Expect(err).ToNot(HaveOccurred())

		err = runner.Start()
		Expect(err).ToNot(HaveOccurred())
	}

	AfterEach(func() {
		err := runner.DestroyContainers()
		Expect(err).ToNot(HaveOccurred())

		restartServer()
	})

	It("retains the container list", func() {
		restartServer()

		handles := getContainerHandles()
		Expect(handles).To(ContainElement(container.Handle()))
	})

	Describe("a started job", func() {
		It("continues to stream", func() {
			processID, runStream, err := container.Run(warden.ProcessSpec{
				Script: "while true; do echo hi; sleep 0.5; done",
			})

			Expect(err).ToNot(HaveOccurred())

			restartServer()

			Eventually(runStream).Should(BeClosed())

			stream, err := container.Attach(processID)
			Expect(err).ToNot(HaveOccurred())

			var chunk warden.ProcessStream
			Eventually(stream).Should(Receive(&chunk))
			Expect(chunk.Data).To(ContainSubstring("hi\n"))
		})

		It("does not have its job ID repeated", func() {
			processID1, _, err := container.Run(warden.ProcessSpec{
				Script: "while true; do echo hi; sleep 0.5; done",
			})
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			processID2, _, err := container.Run(warden.ProcessSpec{
				Script: "while true; do echo hi; sleep 0.5; done",
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(processID1).ToNot(Equal(processID2))
		})

		Context("that prints monotonously increasing output", func() {
			It("does not duplicate its output on reconnect", func(done Done) {
				receivedNumbers := make(chan int, 2048)

				processID, _, err := container.Run(warden.ProcessSpec{
					Script: "for i in $(seq 10); do echo $i; sleep 0.5; done; echo goodbye; while true; do sleep 1; done",
				})
				Expect(err).ToNot(HaveOccurred())

				stream, err := container.Attach(processID)
				Expect(err).ToNot(HaveOccurred())

				go streamNumbersTo(receivedNumbers, stream)

				time.Sleep(500 * time.Millisecond)

				restartServer()

				stream, err = container.Attach(processID)
				Expect(err).ToNot(HaveOccurred())

				go streamNumbersTo(receivedNumbers, stream)

				lastNum := 0
				for num := range receivedNumbers {
					Expect(num).To(BeNumerically(">", lastNum))
					lastNum = num
				}

				close(done)
			}, 10.0)
		})
	})

	Describe("a memory limit", func() {
		It("is still enforced", func() {
			err := container.LimitMemory(warden.MemoryLimits{32 * 1024 * 1024})
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			_, stream, err := container.Run(warden.ProcessSpec{
				Script: "exec ruby -e '$stdout.sync = true; puts :hello; puts (\"x\" * 64 * 1024 * 1024).size; puts :goodbye; exit 42'",
			})
			Expect(err).ToNot(HaveOccurred())

			// cgroups OOM killer seems to leave no trace of the process;
			// there's no exit status indicator, so just assert that the one
			// we tried to exit with after over-allocating is not seen

			stdout, _, exitStatus := readUntilExit(stream)
			Expect(stdout).To(Equal("hello\n"))
			Expect(exitStatus).ToNot(Equal(uint32(42)))
		})
	})

	Describe("a container's active job", func() {
		It("is still tracked", func() {
			processID, _, err := container.Run(warden.ProcessSpec{
				Script: "while true; do echo hi; sleep 0.5; done",
			})
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.ProcessIDs).To(ContainElement(uint32(processID)))
		})
	})

	Describe("a container's list of events", func() {
		It("is still reported", func() {
			err := container.LimitMemory(warden.MemoryLimits{4 * 1024 * 1024})
			Expect(err).ToNot(HaveOccurred())

			// trigger 'out of memory' event
			_, stream, err := container.Run(warden.ProcessSpec{
				Script: "exec ruby -e '$stdout.sync = true; puts :hello; puts (\"x\" * 5 * 1024 * 1024).size; puts :goodbye; exit 42'",
			})
			Expect(err).ToNot(HaveOccurred())

			for _ = range stream {
				// wait until process exits
			}

			Eventually(func() []string {
				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				return info.Events
			}).Should(ContainElement("out of memory"))

			restartServer()

			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Events).To(ContainElement("out of memory"))
		})
	})

	Describe("a container's properties", func() {
		It("are retained", func() {
			containerWithProperties, err := client.Create(warden.ContainerSpec{
				Properties: warden.Properties{
					"foo": "bar",
				},
			})
			Expect(err).ToNot(HaveOccurred())

			info, err := containerWithProperties.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.Properties["foo"]).To(Equal("bar"))

			restartServer()

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

			restartServer()

			info, err = container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("active"))

			err = container.Stop(false)
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			info, err = container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("stopped"))
		})
	})

	Describe("a container's network", func() {
		It("does not get reused", func() {
			infoA, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			newContainer, err := client.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			infoB, err := newContainer.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(infoA.HostIP).ToNot(Equal(infoB.HostIP))
			Expect(infoA.ContainerIP).ToNot(Equal(infoB.ContainerIP))
		})
	})

	Describe("a container's mapped port", func() {
		It("does not get reused", func() {
			netInAHost, netInAContainer, err := container.NetIn(0, 0)
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			containerB, err := client.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			netInBHost, netInBContainer, err := containerB.NetIn(0, 0)
			Expect(err).ToNot(HaveOccurred())

			Expect(netInAHost).ToNot(Equal(netInBHost))
			Expect(netInAContainer).ToNot(Equal(netInBContainer))
		})
	})

	Describe("a container's user", func() {
		It("does not get reused", func() {
			idA := ""
			idB := ""

			_, streamA, err := container.Run(warden.ProcessSpec{
				Script: "id -u",
			})
			Expect(err).ToNot(HaveOccurred())

			for chunk := range streamA {
				idA += string(chunk.Data)
			}

			restartServer()

			otherContainer, err := client.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			_, streamB, err := otherContainer.Run(warden.ProcessSpec{
				Script: "id -u",
			})
			Expect(err).ToNot(HaveOccurred())

			for chunk := range streamB {
				idB += string(chunk.Data)
			}

			Expect(idA).ToNot(Equal(idB))
		})
	})

	Describe("a container's grace time", func() {
		BeforeEach(func() {
			err := runner.Stop()
			Expect(err).ToNot(HaveOccurred())

			err = runner.Start("--containerGraceTime", "5s")
			Expect(err).ToNot(HaveOccurred())
		})

		It("is still enforced", func() {
			container, err := client.Create(warden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			restartServer()

			Expect(getContainerHandles()).To(ContainElement(container.Handle()))
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
