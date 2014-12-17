package lifecycle_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/garden/api"
	gclient "github.com/cloudfoundry-incubator/garden/client"
	gconn "github.com/cloudfoundry-incubator/garden/client/connection"
)

var _ = Describe("When nested", func() {
	var container api.Container
	var nestedGardenAddress string

	nestedRootfsPath := os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS")
	if nestedRootfsPath == "" {
		log.Println("GARDEN_NESTABLE_TEST_ROOTFS undefined; skipping nesting test")
		return
	}

	BeforeEach(func() {
		var err error
		client = startGarden()

		tmpdir, err := ioutil.TempDir("", "nested-garden-test")
		Ω(err).ShouldNot(HaveOccurred())

		absoluteBinPath, err := filepath.Abs(binPath)
		Ω(err).ShouldNot(HaveOccurred())

		container, err = client.Create(api.ContainerSpec{
			RootFSPath: nestedRootfsPath,
			// only privileged containers support nesting
			Privileged: true,
			BindMounts: []api.BindMount{
				{
					SrcPath: filepath.Dir(gardenBin),
					DstPath: "/home/vcap/bin/",
					Mode:    api.BindMountModeRO,
				},
				{
					SrcPath: absoluteBinPath,
					DstPath: "/home/vcap/binpath/bin",
					Mode:    api.BindMountModeRO,
				},
				{
					SrcPath: filepath.Join(absoluteBinPath, "..", "skeleton"),
					DstPath: "/home/vcap/binpath/skeleton",
					Mode:    api.BindMountModeRO,
				},
				{
					SrcPath: rootFSPath,
					DstPath: "/home/vcap/rootfs",
					Mode:    api.BindMountModeRO,
				},
				{
					SrcPath: tmpdir,
					DstPath: "/tmp/nested/",
					Mode:    api.BindMountModeRW,
				},
			},
		})
		Ω(err).ShouldNot(HaveOccurred())

		nestedServerOutput := gbytes.NewBuffer()

		// start nested garden, again need to be root
		_, err = container.Run(api.ProcessSpec{
			Path: "sh",
			User: "root",
			Dir:  "/home/vcap",
			Args: []string{
				"-c",
				`
				mkdir /tmp/overlays /tmp/containers /tmp/snapshots /tmp/graph;
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
				`,
			},
		}, api.ProcessIO{
			Stdout: io.MultiWriter(nestedServerOutput, gexec.NewPrefixedWriter("\x1b[32m[o]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter)),
			Stderr: gexec.NewPrefixedWriter("\x1b[91m[e]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter),
		})

		info, err := container.Info()
		Ω(err).ShouldNot(HaveOccurred())

		nestedGardenAddress = fmt.Sprintf("%s:7778", info.ContainerIP)
		Eventually(nestedServerOutput).Should(gbytes.Say("garden-linux.started"))
	})

	AfterEach(func() {
		Ω(client.Destroy(container.Handle())).Should(Succeed())
	})

	It("can start a nested garden-linux and run a container inside it", func() {
		nestedClient := gclient.New(gconn.New("tcp", nestedGardenAddress))
		nestedContainer, err := nestedClient.Create(api.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		nestedOutput := gbytes.NewBuffer()
		_, err = nestedContainer.Run(api.ProcessSpec{
			Path: "/bin/echo",
			Args: []string{
				"I am nested!",
			},
		}, api.ProcessIO{Stdout: nestedOutput, Stderr: nestedOutput})
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(nestedOutput).Should(gbytes.Say("I am nested!"))
	})
})
