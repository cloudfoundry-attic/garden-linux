package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Garden startup flags", func() {

	var debugAddr string

	BeforeEach(func() {
		debugAddr = fmt.Sprintf("0.0.0.0:%d", 15000+GinkgoParallelNode())
	})

	Context("when starting without the --debugAddr flag", func() {
		BeforeEach(func() {
			client = startGarden()
		})

		It("does not expose the pprof debug endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
			Expect(err).To(HaveOccurred())
		})

		It("does not expose the log level adjustment endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d debug", debugAddr))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when started with the --maxContainers flag", func() {
		Context("when maxContainers is lower than the subnet pool capacity", func() {
			BeforeEach(func() {
				client = startGarden("--maxContainers", "1")
			})

			Context("when getting the capacity", func() {
				It("returns the maxContainers flag value", func() {
					capacity, err := client.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(1)))
				})
			})
		})

		Context("when maxContainers is higher than the subnet pool capacity", func() {
			BeforeEach(func() {
				client = startGarden("--maxContainers", "1000")
			})

			Context("when getting the capacity", func() {
				It("returns the capacity of the subnet pool", func() {
					capacity, err := client.Capacity()
					Expect(err).ToNot(HaveOccurred())
					Expect(capacity.MaxContainers).To(Equal(uint64(64)))
				})
			})
		})
	})

	Context("when starting with the --debugAddr flag", func() {
		BeforeEach(func() {
			client = startGarden("--debugAddr", debugAddr)
		})

		It("exposes the pprof debug endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
			Expect(err).ToNot(HaveOccurred())
		})

		It("exposes the log level adjustment endpoint", func() {
			_, err := http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d debug", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d info", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d error", debugAddr))
			Expect(err).ToNot(HaveOccurred())

			_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d fatal", debugAddr))
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("vars", func() {
			var (
				diskLimits garden.DiskLimits
				container  garden.Container
				vars       map[string]interface{}
			)

			BeforeEach(func() {
				diskLimits = garden.DiskLimits{
					ByteHard: 10 * 1024 * 1024,
					Scope:    garden.DiskLimitScopeExclusive,
				}
			})

			JustBeforeEach(func() {
				var err error

				container, err = client.Create(garden.ContainerSpec{
					Limits: garden.Limits{
						Disk: diskLimits,
					},
					RootFSPath: "docker:///busybox",
				})
				Expect(err).NotTo(HaveOccurred())

				response, err := http.Get(fmt.Sprintf("http://%s/debug/vars", debugAddr))
				Expect(err).ToNot(HaveOccurred())

				contents, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())

				vars = make(map[string]interface{})
				Expect(json.Unmarshal(contents, &vars)).To(Succeed())
			})

			AfterEach(func() {
				Expect(client.Destroy(container.Handle())).To(Succeed())
			})

			It("exposes the number of loop devices", func() {
				Expect(vars["loopDevices"]).To(BeNumerically(">=", float64(1)))
			})

			It("exposes the number of depot directories", func() {
				Expect(vars["depotDirs"]).To(Equal(float64(1)))
			})

			It("exposes the number of backing stores", func() {
				Expect(vars["backingStores"]).To(Equal(float64(1)))
			})

			Context("when the container does not have a limit", func() {
				BeforeEach(func() {
					diskLimits = garden.DiskLimits{}
				})

				It("should not have any backing stores", func() {
					Expect(vars["depotDirs"]).To(Equal(float64(1)))
					Expect(vars["backingStores"]).To(Equal(float64(0)))
				})
			})
		})
	})

	Describe("--enableGraphCleanup", func() {
		var (
			args       []string
			layersPath string
			diffPath   string
			mntPath    string
		)

		graphDirShouldBeEmpty := func() {
			files, err := ioutil.ReadDir(layersPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(0))

			files, err = ioutil.ReadDir(diffPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(0))

			files, err = ioutil.ReadDir(mntPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(0))
		}

		graphDirShouldNotBeEmpty := func() {
			files, err := ioutil.ReadDir(layersPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).NotTo(HaveLen(0))

			files, err = ioutil.ReadDir(diffPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).NotTo(HaveLen(0))

			files, err = ioutil.ReadDir(mntPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).NotTo(HaveLen(0))
		}

		BeforeEach(func() {
			args = []string{}
		})

		JustBeforeEach(func() {
			client = startGarden(args...)

			layersPath = path.Join(client.GraphPath, "aufs", "layers")
			diffPath = path.Join(client.GraphPath, "aufs", "diff")
			mntPath = path.Join(client.GraphPath, "aufs", "mnt")

			container, err := client.Create(garden.ContainerSpec{
				RootFSPath: "docker:///busybox",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(client.Destroy(container.Handle())).To(Succeed())
		})

		Context("when starting without the flag", func() {
			BeforeEach(func() {
				args = append(args, "-enableGraphCleanup=false")
			})

			It("does NOT clean up the graph directory", func() {
				graphDirShouldNotBeEmpty()
			})
		})

		Context("when starting with the flag", func() {
			BeforeEach(func() {
				args = append(args, "-enableGraphCleanup=true")
			})

			It("cleans up the graph directory", func() {
				graphDirShouldBeEmpty()
			})

			Context("when there are other rootfs layers in the graph dir", func() {
				BeforeEach(func() {
					args = append(args, "-persistentImage", "docker:///busybox")
				})

				JustBeforeEach(func() {
					restartGarden() // restart with persistent image list empty
					graphDirShouldNotBeEmpty()
				})

				It("cleans up the graph directory", func() {
					anotherContainer, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())

					Expect(client.Destroy(anotherContainer.Handle())).To(Succeed())
					graphDirShouldBeEmpty()
				})

				It("does not clean up layers that are in use", func() {
					c1, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())

					c2, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())
					Expect(client.Destroy(c2.Handle())).To(Succeed())

					graphDirShouldNotBeEmpty()

					Expect(client.Destroy(c1.Handle())).To(Succeed())
					graphDirShouldBeEmpty()
				})
			})
		})
	})

	Describe("--persistentImage", func() {
		var (
			layersPath       string
			diffPath         string
			mntPath          string
			persistentImages []string
		)

		itDeletesTheRootFS := func() {
			It("deletes the rootfs", func() {
				layers, err := ioutil.ReadDir(layersPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(layers).To(HaveLen(0))

				diffs, err := ioutil.ReadDir(diffPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(diffs).To(HaveLen(0))

				mnts, err := ioutil.ReadDir(diffPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(mnts).To(HaveLen(0))
			})
		}

		BeforeEach(func() {
			persistentImages = []string{}
		})

		JustBeforeEach(func() {
			args := []string{}
			for _, image := range persistentImages {
				args = append(args, "--persistentImage", image)
			}
			client = startGarden(args...)

			layersPath = path.Join(client.GraphPath, "aufs", "layers")
			diffPath = path.Join(client.GraphPath, "aufs", "diff")
			mntPath = path.Join(client.GraphPath, "aufs", "mnt")
		})

		Context("when set", func() {
			var (
				imageLayersAmt int
				diffLayersAmt  int
				mntLayersAmt   int
			)

			itKeepsTheRootFS := func(containersAmt int) {
				It("keeps the rootfs", func() {
					layerFiles, err := ioutil.ReadDir(layersPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(len(layerFiles)).To(Equal(imageLayersAmt - containersAmt)) // should have deleted the container layers, only

					diffFiles, err := ioutil.ReadDir(diffPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(len(diffFiles)).To(Equal(diffLayersAmt - containersAmt)) // should have deleted the container layers, only

					mntFiles, err := ioutil.ReadDir(mntPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(len(mntFiles)).To(Equal(mntLayersAmt - containersAmt)) // should have deleted the container layers, only
				})
			}

			populateMetrics := func() {
				layerFiles, err := ioutil.ReadDir(layersPath)
				Expect(err).ToNot(HaveOccurred())
				imageLayersAmt = len(layerFiles)

				diffFiles, err := ioutil.ReadDir(diffPath)
				Expect(err).ToNot(HaveOccurred())
				diffLayersAmt = len(diffFiles)

				mntFiles, err := ioutil.ReadDir(mntPath)
				Expect(err).ToNot(HaveOccurred())
				mntLayersAmt = len(mntFiles)
			}

			BeforeEach(func() {
				imageLayersAmt = 0
				diffLayersAmt = 0
				mntLayersAmt = 0
			})

			JustBeforeEach(func() {
				Eventually(client, "30s").Should(gbytes.Say("retain.retained"))
			})

			Context("and local images are used", func() {
				BeforeEach(func() {
					persistentImages = []string{runner.RootFSPath}
				})

				Context("and destroying a container that uses a rootfs from the whitelist", func() {
					JustBeforeEach(func() {
						container, err := client.Create(garden.ContainerSpec{
							RootFSPath: persistentImages[0],
						})
						Expect(err).ToNot(HaveOccurred())

						populateMetrics()

						Expect(client.Destroy(container.Handle())).To(Succeed())
					})

					itKeepsTheRootFS(1)

					Context("which is a symlink", func() {
						BeforeEach(func() {
							Expect(os.MkdirAll("/var/vcap/packages", 0755)).To(Succeed())
							err := exec.Command("ln", "-s", runner.RootFSPath, "/var/vcap/packages/busybox").Run()
							Expect(err).ToNot(HaveOccurred())

							persistentImages = []string{"/var/vcap/packages/busybox"}
						})

						itKeepsTheRootFS(1)
					})
				})
			})

			Context("and docker images are used", func() {
				BeforeEach(func() {
					persistentImages = []string{
						"docker:///busybox",
						"docker:///ubuntu",
						"docker://banana/bananatest",
						"docker:///cloudfoundry/garden-busybox",
					}
				})

				Context("and destroying a container that uses a rootfs from the whitelist", func() {
					JustBeforeEach(func() {
						container, err := client.Create(garden.ContainerSpec{
							RootFSPath: "docker:///busybox",
						})
						Expect(err).ToNot(HaveOccurred())

						container2, err := client.Create(garden.ContainerSpec{
							RootFSPath: "docker:///cloudfoundry/garden-busybox",
						})
						Expect(err).ToNot(HaveOccurred())

						populateMetrics()

						Expect(client.Destroy(container.Handle())).To(Succeed())
						Expect(client.Destroy(container2.Handle())).To(Succeed())
					})

					itKeepsTheRootFS(2)
				})

				Context("and destroying a container that uses a rootfs that is not in the whitelist", func() {
					BeforeEach(func() {
						persistentImages = []string{
							"docker:///busybox",
							"docker:///ubuntu",
							"docker://banana/bananatest",
						}
					})

					JustBeforeEach(func() {
						container, err := client.Create(garden.ContainerSpec{
							RootFSPath: "docker:///cloudfoundry/garden-busybox",
						})
						Expect(err).ToNot(HaveOccurred())

						Expect(client.Destroy(container.Handle())).To(Succeed())
					})

					itDeletesTheRootFS()
				})
			})
		})

		Context("when it is not set", func() {
			Context("and destroying a container", func() {
				JustBeforeEach(func() {
					container, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///busybox",
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(client.Destroy(container.Handle())).To(Succeed())
				})

				itDeletesTheRootFS()
			})
		})
	})
})
