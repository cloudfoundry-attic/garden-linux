package lifecycle_test

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
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

	Context("when destroying a container with a disk limit", func() {
		var tgzPath string

		createContainer := func() (garden.Container, error) {
			container, err := client.Create(garden.ContainerSpec{
				RootFSPath: "docker:///busybox",
				Limits: garden.Limits{
					Disk: garden.DiskLimits{
						ByteHard: 100 * 1024 * 1024,
						Scope:    garden.DiskLimitScopeExclusive,
					},
				},
			})

			return container, err
		}

		streamInDora := func(container garden.Container) error {
			tgz, err := os.Open(tgzPath)
			if err != nil {
				return err
			}

			tarStream, err := gzip.NewReader(tgz)
			if err != nil {
				return err
			}

			err = container.StreamIn(garden.StreamInSpec{
				User:      "root",
				Path:      "/root/dora",
				TarStream: tarStream,
			})

			return err
		}

		destroy := func(handle string) error {
			err := client.Destroy(handle)
			return err
		}

		entriesAmt := func(path string) int {
			entries, err := ioutil.ReadDir(path)
			Expect(err).NotTo(HaveOccurred())

			return len(entries)
		}

		BeforeEach(func() {
			tgzPath = os.Getenv("GARDEN_DORA_PATH")
			if tgzPath == "" {
				Skip("`GARDEN_DORA_PATH` is not set")
			}
		})

		It("should remove the AUFS directories", func() {
			client = startGarden()

			containersAmt := 5

			beforeBsAmt := entriesAmt(filepath.Join(client.GraphPath, "backing_stores"))

			h := make(chan string, containersAmt)
			createErrs := make(chan error, containersAmt)
			destroyErrs := make(chan error, containersAmt)
			for i := 0; i < containersAmt; i++ {
				go func() {
					defer GinkgoRecover()

					container, err := createContainer()
					if err != nil {
						createErrs <- err
						return
					}

					if err := streamInDora(container); err != nil {
						createErrs <- err
						return
					}

					h <- container.Handle()
				}()
			}

			for i := 0; i < containersAmt; i++ {
				select {
				case handle := <-h:
					go func() {
						defer GinkgoRecover()

						destroyErrs <- destroy(handle)
					}()
				case err := <-createErrs:
					Fail(err.Error())
				}
			}

			for i := 0; i < containersAmt; i++ {
				e := <-destroyErrs
				Expect(e).NotTo(HaveOccurred())
			}

			afterBsAmt := entriesAmt(filepath.Join(client.GraphPath, "backing_stores"))
			Expect(afterBsAmt).To(Equal(beforeBsAmt))
			afterDiffAmt := entriesAmt(filepath.Join(client.GraphPath, "aufs", "diff"))
			Expect(afterDiffAmt).To(Equal(0))
			afterMntAmt := entriesAmt(filepath.Join(client.GraphPath, "aufs", "mnt"))
			Expect(afterMntAmt).To(Equal(0))
		})
	})
})
