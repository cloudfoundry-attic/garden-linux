package lifecycle_test

import (
	"runtime/debug"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Process", func() {

	var container garden.Container

	BeforeEach(func() {
		client = startGarden()
		var err error
		container, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("signalling", func() {
		It("a process can be sent SIGTERM immediately after having been started", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				User: "vcap",
				Path: "sh",
				Args: []string{
					"-c",
					`
                sleep 10
                exit 12
                `,
				},
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			err = process.Signal(garden.SignalTerminate)
			Expect(err).ToNot(HaveOccurred())
			Expect(process.Wait()).NotTo(Equal(12))
		})
	})

	startAndWait := func() (garden.Process, <-chan int) {
		buf := gbytes.NewBuffer()
		procIo := garden.ProcessIO{
			Stdout: buf,
			Stderr: buf,
		}
		process, err := container.Run(garden.ProcessSpec{
			User: "vcap",
			Path: "sh",
			Args: []string{"-c", `
				  trap 'echo termed; exit 42' SIGTERM

					while true; do
					  echo waiting
					  sleep 1
					done
				`},
		}, procIo)
		Expect(err).NotTo(HaveOccurred())

		attachedProcess, err := container.Attach(process.ID(), procIo)
		Expect(err).NotTo(HaveOccurred())
		Eventually(buf).Should(gbytes.Say("waiting"))

		exitChan := make(chan int)
		go func(p garden.Process, exited chan<- int) {
			GinkgoRecover()
			status, waitErr := p.Wait()
			Expect(waitErr).NotTo(HaveOccurred())
			exited <- status
		}(attachedProcess, exitChan)

		return attachedProcess, exitChan
	}

	waitForExit := func(p garden.Process, e <-chan int) {
		buf := gbytes.NewBuffer()
		procIo := garden.ProcessIO{
			Stdout: buf,
			Stderr: buf,
		}
		attachedProcess, err := container.Attach(p.ID(), procIo)
		Expect(err).NotTo(HaveOccurred())
		Eventually(buf).Should(gbytes.Say("waiting"))

		Expect(attachedProcess.Signal(garden.SignalTerminate)).To(Succeed())
		select {
		case status := <-e:
			Expect(status).To(Equal(42))
			Eventually(buf).Should(gbytes.Say("termed"))
		case <-time.After(time.Second * 10):
			debug.PrintStack()
			Fail("timed out!")
		}
	}

	It("should not allow process outcomes to interfere with eachother", func() {
		p1, e1 := startAndWait()
		p2, e2 := startAndWait()
		p3, e3 := startAndWait()
		p4, e4 := startAndWait()

		waitForExit(p1, e1)
		waitForExit(p2, e2)
		waitForExit(p4, e4)
		waitForExit(p3, e3)
	})
})
