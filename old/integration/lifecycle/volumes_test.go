package lifecycle_test

import (
	"github.com/cloudfoundry-incubator/garden/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("A volume", func() {
	var (
		container         api.Container
		readOnlyContainer api.Container
	)

	BeforeEach(func() {
		client = startGarden()

		var err error

		container, err = client.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		if readOnlyContainer != nil {
			err := client.Destroy(readOnlyContainer.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	It("can be created and attached to multiple containers", func() {
		volume, err := client.CreateVolume(api.VolumeSpec{
			Handle: "some volume handle",
		})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(volume.Handle()).Should(Equal("some volume handle"))

		// first ensure that the container cannot write to the bridge location
		process, err := container.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/container-shared-mounts/lol"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).ShouldNot(BeZero())

		// bind volume read-write to container
		err = container.BindVolume(volume, api.VolumeBinding{
			Mode:        api.VolumeBindingModeRW,
			Destination: "/tmp/path/in/container",
		})
		Ω(err).ShouldNot(HaveOccurred())

		// make a second container to bind it to
		readOnlyContainer, err = client.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		// read-write can write
		process, err = container.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/path/in/container/rw-before-ro-mount"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).Should(BeZero())

		// bind-mount read-only to second container
		err = readOnlyContainer.BindVolume(volume, api.VolumeBinding{
			Mode:        api.VolumeBindingModeRO,
			Destination: "/tmp/path/in/container",
		})
		Ω(err).ShouldNot(HaveOccurred())

		// read-write can still write
		process, err = container.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/path/in/container/rw-after-ro-mount"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).Should(BeZero())

		// read-only can see writes from read-write
		process, err = readOnlyContainer.Run(api.ProcessSpec{
			Path:       "ls",
			Args:       []string{"/tmp/path/in/container/rw-after-ro-mount"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).Should(BeZero())

		// read-only cannot write
		process, err = readOnlyContainer.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/path/in/container/ro-after-ro-mount"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).ShouldNot(BeZero())
	})
})
