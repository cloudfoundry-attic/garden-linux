package lifecycle_test

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/garden"
	gclient "github.com/cloudfoundry-incubator/garden/client"
	gconn "github.com/cloudfoundry-incubator/garden/client/connection"
)

var _ = XDescribe("When nested", func() {
	nestedRootfsPath := os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS")
	if nestedRootfsPath == "" {
		log.Println("GARDEN_NESTABLE_TEST_ROOTFS undefined; skipping nesting test")
		return
	}

	BeforeEach(func() {
		client = startGarden()
	})

	startNestedGarden := func(mountOverlayOnTmpfs bool) (garden.Container, string) {
		absoluteBinPath, err := filepath.Abs(binPath)
		Ω(err).ShouldNot(HaveOccurred())

		container, err := client.Create(garden.ContainerSpec{
			RootFSPath: nestedRootfsPath,
			// only privileged containers support nesting
			Privileged: true,
			BindMounts: []garden.BindMount{
				{
					SrcPath: filepath.Dir(gardenBin),
					DstPath: "/home/vcap/bin/",
					Mode:    garden.BindMountModeRO,
				},
				{
					SrcPath: absoluteBinPath,
					DstPath: "/home/vcap/binpath/bin",
					Mode:    garden.BindMountModeRO,
				},
				{
					SrcPath: filepath.Join(absoluteBinPath, "..", "skeleton"),
					DstPath: "/home/vcap/binpath/skeleton",
					Mode:    garden.BindMountModeRO,
				},
				{
					SrcPath: rootFSPath,
					DstPath: "/home/vcap/rootfs",
					Mode:    garden.BindMountModeRO,
				},
			},
		})
		Ω(err).ShouldNot(HaveOccurred())

		nestedServerOutput := gbytes.NewBuffer()

		extraMounts := ""
		if mountOverlayOnTmpfs {
			extraMounts = "mount -t tmpfs tmpfs /tmp/overlays"
		}

		// start nested garden, again need to be root
		_, err = container.Run(garden.ProcessSpec{
			Path: "sh",
			User: "root",
			Dir:  "/home/vcap",
			Args: []string{
				"-c",
				fmt.Sprintf(`
				mkdir /tmp/overlays /tmp/containers /tmp/snapshots /tmp/graph;
				%s
				mount -t tmpfs tmpfs /tmp/containers

				./bin/garden-linux \
					-bin /home/vcap/binpath/bin \
					-rootfs /home/vcap/rootfs \
					-depot /tmp/containers \
					-overlays /tmp/overlays \
					-snapshots /tmp/snapshots \
					-graph /tmp/graph \
					-disableQuotas \
					-listenNetwork tcp \
					-listenAddr 0.0.0.0:7778;
				`, extraMounts),
			},
		}, garden.ProcessIO{
			Stdout: io.MultiWriter(nestedServerOutput, gexec.NewPrefixedWriter("\x1b[32m[o]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter)),
			Stderr: gexec.NewPrefixedWriter("\x1b[91m[e]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter),
		})

		info, err := container.Info()
		Ω(err).ShouldNot(HaveOccurred())

		nestedGardenAddress := fmt.Sprintf("%s:7778", info.ContainerIP)
		Eventually(nestedServerOutput, "30s").Should(gbytes.Say("garden-linux.started"))

		return container, nestedGardenAddress
	}

	It("can start a nested garden-linux and run a container inside it", func() {
		container, nestedGardenAddress := startNestedGarden(true)
		defer client.Destroy(container.Handle())

		nestedClient := gclient.New(gconn.New("tcp", nestedGardenAddress))
		nestedContainer, err := nestedClient.Create(garden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		nestedOutput := gbytes.NewBuffer()
		_, err = nestedContainer.Run(garden.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{
				"I am nested!",
			},
		}, garden.ProcessIO{Stdout: nestedOutput, Stderr: nestedOutput})
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(nestedOutput, "30s").Should(gbytes.Say("I am nested!"))
	})

	It("returns helpful error message when depot directory fstype cannot be nested", func() {
		container, nestedGardenAddress := startNestedGarden(false)
		defer client.Destroy(container.Handle())

		nestedClient := gclient.New(gconn.New("tcp", nestedGardenAddress))
		_, err := nestedClient.Create(garden.ContainerSpec{})
		Ω(err).Should(MatchError("overlay.sh: exit status 222, the directories that contain the depot and rootfs must be mounted on a filesystem type that supports aufs or overlayfs"))
	})
})
