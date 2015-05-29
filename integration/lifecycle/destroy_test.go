package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Destroying a container", func() {
	Context("when wshd goes away", func() {
		Context("the container destruction", func() {
			It("succeeds", func() {
				client = startGarden()

				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).ToNot(HaveOccurred())

				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())
				pidFilePath := filepath.Join(info.ContainerPath, "run", "wshd.pid")

				fileContents, err := ioutil.ReadFile(pidFilePath)
				Expect(err).ToNot(HaveOccurred())
				Expect(fileContents).ToNot(BeEmpty())

				var pid int
				n, err := fmt.Sscanf(string(fileContents), "%d", &pid)
				Expect(err).ToNot(HaveOccurred())
				Expect(n).To(Equal(1))

				cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
				cmd.Stdout = GinkgoWriter
				cmd.Stderr = GinkgoWriter
				Expect(cmd.Run()).To(Succeed())

				Expect(client.Destroy(container.Handle())).To(Succeed())
			})
		})
	})
})
