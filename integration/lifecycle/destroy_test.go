package lifecycle_test

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
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

	// Need to make this test work consistently with ginkgo -p. The loop
	// devices is a shared resource and it can be affected by other tests.
	// One potential solution is to check that the loop devices
	// used by this test are gone.
	PContext("when destroying a container with a disk limit", func() {
		var tgzPath string

		BeforeEach(func() {
			tgzPath = os.Getenv("GARDEN_DORA_PATH")
			if tgzPath == "" {
				Skip("`GARDEN_DORA_PATH` is not set")
			}
		})

		It("should remove the AUFS directories", func() {
			client = startGarden()

			var handles []string
			containersAmt := 5

			h := make(chan string, containersAmt)
			errs := make(chan error, containersAmt)
			for i := 0; i < containersAmt; i++ {
				go func() {
					defer GinkgoRecover()

					container, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///busybox",
						Limits: garden.Limits{
							Disk: garden.DiskLimits{
								ByteHard: 100 * 1024 * 1024,
								Scope:    garden.DiskLimitScopeExclusive,
							},
						},
					})
					if err != nil {
						errs <- err
						return
					}

					tgz, err := os.Open(tgzPath)
					if err != nil {
						errs <- err
						return
					}

					tarStream, err := gzip.NewReader(tgz)
					if err != nil {
						errs <- err
						return
					}

					fmt.Println("stream-in-start")
					err = container.StreamIn(garden.StreamInSpec{
						User:      "root",
						Path:      "/root/dora",
						TarStream: tarStream,
					})

					if err != nil {
						errs <- err
						return
					}

					fmt.Println("stream-in-complete")

					h <- container.Handle()
				}()
			}

			for i := 0; i < containersAmt; i++ {
				select {
				case handle := <-h:
					handles = append(handles, handle)
					fmt.Println("created", i)
				case err := <-errs:
					Fail(err.Error())
				}
			}

			buffer := gbytes.NewBuffer()
			cmd := exec.Command("sh", "-c", "losetup -a | wc -l")
			cmd.Stdout = buffer
			Expect(cmd.Run()).To(Succeed())
			pdLoopEntLen, err := strconv.ParseInt(strings.TrimSpace(string(buffer.Contents())), 10, 32)
			Expect(err).NotTo(HaveOccurred())

			entries, err := ioutil.ReadDir(filepath.Join(client.GraphPath, "backing_stores"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(containersAmt))

			entries, err = ioutil.ReadDir(filepath.Join(client.GraphPath, "aufs", "diff"))
			Expect(err).NotTo(HaveOccurred())
			pdDiffEntLen := len(entries)

			entries, err = ioutil.ReadDir(filepath.Join(client.GraphPath, "aufs", "mnt"))
			Expect(err).NotTo(HaveOccurred())
			pdMntEntLen := len(entries)

			errors := make(chan error, containersAmt)
			for i := 0; i < containersAmt; i++ {
				go func(i int, ec chan error) {
					defer GinkgoRecover()
					errors <- client.Destroy(handles[i])
				}(i, errors)
			}

			for i := 0; i < containersAmt; i++ {
				e := <-errors
				Expect(e).NotTo(HaveOccurred())
			}

			buffer = gbytes.NewBuffer()
			cmd = exec.Command("sh", "-c", "losetup -a | wc -l")
			cmd.Stdout = buffer
			Expect(cmd.Run()).To(Succeed())
			adLoopEntLen, err := strconv.ParseInt(strings.TrimSpace(string(buffer.Contents())), 10, 32)
			Expect(err).NotTo(HaveOccurred())
			Expect(adLoopEntLen).To(Equal(pdLoopEntLen - int64(containersAmt)))

			entries, err = ioutil.ReadDir(filepath.Join(client.GraphPath, "backing_stores"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(0))

			entries, err = ioutil.ReadDir(filepath.Join(client.GraphPath, "aufs", "diff"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(pdDiffEntLen - containersAmt))

			entries, err = ioutil.ReadDir(filepath.Join(client.GraphPath, "aufs", "mnt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(pdMntEntLen - containersAmt))
		})
	})
})
