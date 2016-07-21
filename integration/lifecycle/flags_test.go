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

	Describe("graph cleanup flags", func() {
		var (
			layersPath           string
			diffPath             string
			mntPath              string
			nonDefaultRootfsPath string
			args                 []string
			persistentImages     []string
		)

		numLayersInGraph := func() int {
			layerFiles, err := ioutil.ReadDir(layersPath)
			Expect(err).ToNot(HaveOccurred())
			diffFiles, err := ioutil.ReadDir(diffPath)
			Expect(err).ToNot(HaveOccurred())
			mntFiles, err := ioutil.ReadDir(mntPath)
			Expect(err).ToNot(HaveOccurred())

			numLayerFiles := len(layerFiles)
			Expect(numLayerFiles).To(Equal(len(diffFiles)))
			Expect(numLayerFiles).To(Equal(len(mntFiles)))
			return numLayerFiles
		}

		expectLayerCountAfterGraphCleanupToBe := func(layerCount int) {
			nonPersistantRootfsContainer, err := client.Create(garden.ContainerSpec{
				RootFSPath: nonDefaultRootfsPath,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(client.Destroy(nonPersistantRootfsContainer.Handle())).To(Succeed())
			Expect(numLayersInGraph()).To(Equal(layerCount + 2)) // +2 for the layers created for the nondefaultrootfs container
		}

		BeforeEach(func() {
			var err error
			nonDefaultRootfsPath, err = ioutil.TempDir("", "tmpRootfs")
			Expect(err).ToNot(HaveOccurred())
		})

		JustBeforeEach(func() {
			for _, image := range persistentImages {
				args = append(args, "--persistentImage", image)
			}
			client = startGarden(args...)

			layersPath = path.Join(client.GraphPath, "aufs", "layers")
			diffPath = path.Join(client.GraphPath, "aufs", "diff")
			mntPath = path.Join(client.GraphPath, "aufs", "mnt")
		})

		AfterEach(func() {
			Expect(os.RemoveAll(nonDefaultRootfsPath)).To(Succeed())
		})

		Describe("--enableGraphCleanup", func() {

			JustBeforeEach(func() {
				container, err := client.Create(garden.ContainerSpec{
					RootFSPath: "docker:///busybox",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(client.Destroy(container.Handle())).To(Succeed())
			})

			Context("when starting without the flag", func() {
				BeforeEach(func() {
					args = []string{"-enableGraphCleanup=false"}
				})

				It("does NOT clean up the graph directory on create", func() {
					initialNumberOfLayers := numLayersInGraph()
					anotherContainer, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())

					Expect(numLayersInGraph()).To(BeNumerically(">", initialNumberOfLayers), "after creation, should NOT have deleted anything")
					Expect(client.Destroy(anotherContainer.Handle())).To(Succeed())
				})
			})

			Context("when starting with the flag", func() {
				BeforeEach(func() {
					args = []string{"-enableGraphCleanup=true"}
				})

				Context("when there are other rootfs layers in the graph dir", func() {
					BeforeEach(func() {
						args = append(args, "-persistentImage", "docker:///busybox")
					})

					It("cleans up the graph directory on container creation (and not on destruction)", func() {
						restartGarden("-enableGraphCleanup=true") // restart with persistent image list empty
						Expect(numLayersInGraph()).To(BeNumerically(">", 0))

						anotherContainer, err := client.Create(garden.ContainerSpec{})
						Expect(err).ToNot(HaveOccurred())

						Expect(numLayersInGraph()).To(Equal(3), "after creation, should have deleted everything other than the default rootfs, uid translation layer and container layer")
						Expect(client.Destroy(anotherContainer.Handle())).To(Succeed())
						Expect(numLayersInGraph()).To(Equal(2), "should not garbage collect parent layers on destroy")
					})
				})
			})
		})

		Describe("--persistentImage", func() {
			BeforeEach(func() {
				args = []string{"-enableGraphCleanup=true"}
			})

			Context("when set", func() {
				JustBeforeEach(func() {
					Eventually(client, "30s").Should(gbytes.Say("retain.retained"))
				})

				Context("and local images are used", func() {
					BeforeEach(func() {
						persistentImages = []string{runner.RootFSPath}
					})

					Describe("graph cleanup for a rootfs on the whitelist", func() {
						It("keeps the rootfs in the graph", func() {
							container, err := client.Create(garden.ContainerSpec{
								RootFSPath: persistentImages[0],
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(client.Destroy(container.Handle())).To(Succeed())

							expectLayerCountAfterGraphCleanupToBe(2)
						})

						Context("which is a symlink", func() {
							BeforeEach(func() {
								Expect(os.MkdirAll("/var/vcap/packages", 0755)).To(Succeed())
								err := exec.Command("ln", "-s", runner.RootFSPath, "/var/vcap/packages/busybox").Run()
								Expect(err).ToNot(HaveOccurred())

								persistentImages = []string{"/var/vcap/packages/busybox"}
							})

							It("keeps the rootfs in the graph", func() {
								container, err := client.Create(garden.ContainerSpec{
									RootFSPath: persistentImages[0],
								})
								Expect(err).ToNot(HaveOccurred())
								Expect(client.Destroy(container.Handle())).To(Succeed())

								expectLayerCountAfterGraphCleanupToBe(2)
							})
						})
					})

					Describe("graph cleanup for a rootfs not on the whitelist", func() {
						It("cleans up all unused images from the graph", func() {
							container, err := client.Create(garden.ContainerSpec{
								RootFSPath: nonDefaultRootfsPath,
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(client.Destroy(container.Handle())).To(Succeed())

							expectLayerCountAfterGraphCleanupToBe(0)
						})
					})
				})

				Context("and docker images are used", func() {
					BeforeEach(func() {
						persistentImages = []string{
							"docker:///busybox",
							"docker:///ubuntu",
							"docker://banana/bananatest",
						}
					})

					Describe("graph cleanup for a rootfs on the whitelist", func() {
						It("keeps the rootfs in the graph", func() {
							numLayersBeforeDockerPull := numLayersInGraph()
							container, err := client.Create(garden.ContainerSpec{
								RootFSPath: persistentImages[0],
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(client.Destroy(container.Handle())).To(Succeed())
							numLayersInImage := numLayersInGraph() - numLayersBeforeDockerPull

							expectLayerCountAfterGraphCleanupToBe(numLayersInImage)
						})
					})

					Describe("graph cleanup for a rootfs not on the whitelist", func() {
						It("cleans up all unused images from the graph", func() {
							container, err := client.Create(garden.ContainerSpec{
								RootFSPath: "docker:///cfgarden/garden-busybox",
							})
							Expect(err).ToNot(HaveOccurred())
							Expect(client.Destroy(container.Handle())).To(Succeed())

							expectLayerCountAfterGraphCleanupToBe(0)
						})
					})
				})
			})

			Context("when it is not set", func() {
				BeforeEach(func() {
					persistentImages = []string{}
				})

				It("cleans up all unused images from the graph", func() {
					defaultRootfsContainer, err := client.Create(garden.ContainerSpec{})
					Expect(err).ToNot(HaveOccurred())

					nonDefaultRootfsContainer, err := client.Create(garden.ContainerSpec{
						RootFSPath: nonDefaultRootfsPath,
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(client.Destroy(defaultRootfsContainer.Handle())).To(Succeed())
					Expect(client.Destroy(nonDefaultRootfsContainer.Handle())).To(Succeed())

					expectLayerCountAfterGraphCleanupToBe(0)
				})
			})
		})
	})
})
