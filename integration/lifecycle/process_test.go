package lifecycle_test

import (
	"os"
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
		container, err = client.Create(garden.ContainerSpec{
			RootFSPath: os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS"),
		})
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

	It("wait returns when all children of the process have exited", func() {
		buffer := gbytes.NewBuffer()
		process, err := container.Run(garden.ProcessSpec{
			User: "vcap",
			Path: "/bin/bash",
			Args: []string{"-c", `

				  cleanup ()
				  {
				  	kill $child_pid
				  	exit 42
				  }

				  trap cleanup TERM
				  echo trapping

				  sleep 1000 &
				  child_pid=$!
				  wait
				`},
		}, garden.ProcessIO{Stdout: buffer})
		Expect(err).NotTo(HaveOccurred())

		exitChan := make(chan int)
		go func(p garden.Process, exited chan<- int) {
			GinkgoRecover()
			status, waitErr := p.Wait()
			Expect(waitErr).NotTo(HaveOccurred())
			exited <- status
		}(process, exitChan)

		Eventually(buffer).Should(gbytes.Say("trapping"))

		Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
		select {
		case status := <-exitChan:
			Expect(status).To(Equal(42))
		case <-time.After(time.Second * 10):
			debug.PrintStack()
			Fail("timed out!")
		}
	})

	FIt("wait does not block when a child of the process has not exited", func() {
		buffer := gbytes.NewBuffer()
		process, err := container.Run(garden.ProcessSpec{
			User: "vcap",
			Path: "/bin/bash",
			Args: []string{"-c", `
					cleanup ()
					{
						exit 42
					}

					trap cleanup TERM
					sleep 100000 &
					disown

					echo trapping

					sleep 100000
				`},
		}, garden.ProcessIO{Stdout: buffer})
		Expect(err).NotTo(HaveOccurred())

		exitChan := make(chan int)
		go func(p garden.Process, exited chan<- int) {
			GinkgoRecover()
			status, waitErr := p.Wait()
			Expect(waitErr).NotTo(HaveOccurred())
			exited <- status
		}(process, exitChan)

		Eventually(buffer).Should(gbytes.Say("trapping"))

		Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
		select {
		case status := <-exitChan:
			Expect(status).To(Equal(42))
		case <-time.After(time.Second * 3):
			Fail("Wait should not block when a child has not exited")
		}
	})

})
