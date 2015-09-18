package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/cloudfoundry-incubator/garden"
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
	})

	Describe("--persistentImageList", func() {
		var layersPath string

		Context("when set", func() {
			BeforeEach(func() {
				client = startGarden("--persistentImageList", "docker:///busybox,docker:///ubuntu,docker://banana/bananatest,docker:///cloudfoundry/with-volume")
				layersPath = path.Join(client.GraphPath, "btrfs", "subvolumes")

				Eventually(client, "30s").Should(gbytes.Say("retain.retained"))
			})

			Context("and destroying a container that uses a rootfs from the whitelist", func() {
				var imageLayersAmt int

				BeforeEach(func() {
					container, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///busybox",
					})
					Expect(err).ToNot(HaveOccurred())

					container2, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///cloudfoundry/with-volume",
					})
					Expect(err).ToNot(HaveOccurred())

					files, err := ioutil.ReadDir(layersPath)
					Expect(err).ToNot(HaveOccurred())
					imageLayersAmt = len(files)

					Expect(client.Destroy(container.Handle())).To(Succeed())
					Expect(client.Destroy(container2.Handle())).To(Succeed())
				})

				It("keeps the rootfs", func() {
					files, err := ioutil.ReadDir(layersPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(files).To(HaveLen(imageLayersAmt - 2)) // should have deleted the container layers, only
				})
			})

			Context("and destroying a container that uses a rootfs that is not in the whitelist", func() {
				BeforeEach(func() {
					container, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///cloudfoundry/garden-busybox",
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(client.Destroy(container.Handle())).To(Succeed())
				})

				It("deletes the rootfs", func() {
					files, err := ioutil.ReadDir(layersPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(files).To(HaveLen(0))
				})
			})
		})

		Context("when it is not set", func() {
			BeforeEach(func() {
				client = startGarden()
				layersPath = path.Join(client.GraphPath, "btrfs", "subvolumes")
			})

			Context("and destroying a container", func() {
				BeforeEach(func() {
					container, err := client.Create(garden.ContainerSpec{
						RootFSPath: "docker:///busybox",
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(client.Destroy(container.Handle())).To(Succeed())
				})

				It("deletes the rootfs", func() {
					files, err := ioutil.ReadDir(layersPath)
					Expect(err).ToNot(HaveOccurred())

					Expect(files).To(HaveLen(0))
				})
			})
		})
	})
})
