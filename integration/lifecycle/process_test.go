package lifecycle_test

import (
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
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("signalling", func() {

		It("a process can be sent SIGTERM immediately after having been started", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{
					"-c",
					`
                trap 'exit 42' SIGTERM

                sleep 2
                exit 12
                `,
				},
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Ω(err).ShouldNot(HaveOccurred())

			err = process.Signal(garden.SignalTerminate)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).Should(Equal(42))
		})

	})

})
