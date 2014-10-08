package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

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

		readOnlyContainer = nil
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Ω(err).ShouldNot(HaveOccurred())

		if readOnlyContainer != nil {
			err := client.Destroy(readOnlyContainer.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	It("can be created with a handle", func() {
		volume, err := client.CreateVolume(api.VolumeSpec{
			Handle: "some volume handle",
		})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(volume.Handle()).Should(Equal("some volume handle"))
	})

	It("does not allow the container to write to the host", func() {
		// this is more of a security check than a behavioral test of volumes.

		process, err := container.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/container-shared-mounts/lol"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).ShouldNot(BeZero())

		_, err = client.CreateVolume(api.VolumeSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		process, err = container.Run(api.ProcessSpec{
			Path:       "touch",
			Args:       []string{"/tmp/container-shared-mounts/lol"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).ShouldNot(BeZero())
	})

	It("can be safely unmounted in the container", func() {
		// this covers a weird case, possibly a bad actor container, wherein
		// the shared mounts get unmounted in the container.
		//
		// the host must be durable to this.
		//
		// this effectively tests that the shared mount is made a 'slave' to the
		// container, which prevents mount/umount changes from propagating up to
		// the host.
		//
		// the failure will occur in `AfterEach`

		volume, err := client.CreateVolume(api.VolumeSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		// bind volume read-write to container
		err = container.BindVolume(volume, api.VolumeBinding{
			Mode:        api.VolumeBindingModeRW,
			Destination: "/tmp/path/in/container",
		})
		Ω(err).ShouldNot(HaveOccurred())

		process, err := container.Run(api.ProcessSpec{
			Path:       "umount",
			Args:       []string{"-l", "/tmp/container-shared-mounts"},
			Privileged: true,
		}, api.ProcessIO{})
		Ω(err).ShouldNot(HaveOccurred())
		Ω(process.Wait()).Should(BeZero())
	})

	It("can be created and attached to multiple containers", func() {
		volume, err := client.CreateVolume(api.VolumeSpec{})
		Ω(err).ShouldNot(HaveOccurred())

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
		process, err := container.Run(api.ProcessSpec{
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

	Context("when a volume is created, mapped to a directory on the host", func() {
		var tmpdir string

		BeforeEach(func() {
			var err error

			tmpdir, err = ioutil.TempDir("", "host-dir")
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(tmpdir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("can be bound read-write to a container", func() {
			volume, err := client.CreateVolume(api.VolumeSpec{
				HostPath: tmpdir,
			})
			Ω(err).ShouldNot(HaveOccurred())

			err = container.BindVolume(volume, api.VolumeBinding{
				Mode:        api.VolumeBindingModeRW,
				Destination: "/tmp/path/in/container",
			})
			Ω(err).ShouldNot(HaveOccurred())

			process, err := container.Run(api.ProcessSpec{
				Path:       "touch",
				Args:       []string{"/tmp/path/in/container/some-file"},
				Privileged: true,
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).Should(BeZero())

			_, err = os.Stat(filepath.Join(tmpdir, "some-file"))
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("can be bound read-only to a container", func() {
			volume, err := client.CreateVolume(api.VolumeSpec{
				HostPath: tmpdir,
			})
			Ω(err).ShouldNot(HaveOccurred())

			err = container.BindVolume(volume, api.VolumeBinding{
				Mode:        api.VolumeBindingModeRO,
				Destination: "/tmp/path/in/container",
			})
			Ω(err).ShouldNot(HaveOccurred())

			process, err := container.Run(api.ProcessSpec{
				Path:       "touch",
				Args:       []string{"/tmp/path/in/container/some-file"},
				Privileged: true,
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).ShouldNot(BeZero())

			process, err = container.Run(api.ProcessSpec{
				Path:       "ls",
				Args:       []string{"/tmp/path/in/container/some-file"},
				Privileged: true,
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).ShouldNot(BeZero())

			file, err := os.Create(filepath.Join(tmpdir, "some-file"))
			Ω(err).ShouldNot(HaveOccurred())

			err = file.Close()
			Ω(err).ShouldNot(HaveOccurred())

			process, err = container.Run(api.ProcessSpec{
				Path:       "ls",
				Args:       []string{"/tmp/path/in/container/some-file"},
				Privileged: true,
			}, api.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(process.Wait()).Should(BeZero())
		})
	})
})
