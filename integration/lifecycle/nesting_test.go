package lifecycle_test

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"io/ioutil"

	"github.com/cloudfoundry-incubator/garden"
	gclient "github.com/cloudfoundry-incubator/garden/client"
	gconn "github.com/cloudfoundry-incubator/garden/client/connection"
)

var _ = Describe("When nested", func() {
	nestedRootfsPath := os.Getenv("GARDEN_NESTABLE_TEST_ROOTFS")
	if nestedRootfsPath == "" {
		log.Println("GARDEN_NESTABLE_TEST_ROOTFS undefined; skipping nesting test")
		return
	}

	BeforeEach(func() {
		client = startGarden()
	})

	startNestedGarden := func() (garden.Container, string) {
		absoluteBinPath, err := filepath.Abs(binPath)
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

		nestedServerOutput := gbytes.NewBuffer()

		// start nested garden, again need to be root
		_, err = container.Run(garden.ProcessSpec{
			Path: "sh",
			User: "root",
			Dir:  "/home/vcap",
			Args: []string{
				"-c",
				fmt.Sprintf(`
				set -e
				mkdir /tmp/containers /tmp/snapshots /tmp/graph;
				mount -t tmpfs tmpfs /tmp/containers

				./bin/garden-linux \
					-bin /home/vcap/binpath/bin \
					-rootfs /home/vcap/rootfs \
					-depot /tmp/containers \
					-snapshots /tmp/snapshots \
					-graph /tmp/graph \
					-tag n \
					-disableQuotas \
					-listenNetwork tcp \
					-listenAddr 0.0.0.0:7778
				`),
			},
		}, garden.ProcessIO{
			Stdout: io.MultiWriter(nestedServerOutput, gexec.NewPrefixedWriter("\x1b[32m[o]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter)),
			Stderr: gexec.NewPrefixedWriter("\x1b[91m[e]\x1b[34m[nested-garden-linux]\x1b[0m ", GinkgoWriter),
		})

		info, err := container.Info()
		Expect(err).ToNot(HaveOccurred())

		nestedGardenAddress := fmt.Sprintf("%s:7778", info.ContainerIP)
		Eventually(nestedServerOutput, "60s").Should(gbytes.Say("garden-linux.started"))

		return container, nestedGardenAddress
	}

	It("can start a nested garden-linux and run a container inside it", func() {
		container, nestedGardenAddress := startNestedGarden()
		defer func() {
			Expect(client.Destroy(container.Handle())).To(Succeed())
		}()

		nestedClient := gclient.New(gconn.New("tcp", nestedGardenAddress))
		nestedContainer, err := nestedClient.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())

		nestedOutput := gbytes.NewBuffer()
		_, err = nestedContainer.Run(garden.ProcessSpec{
			User: "vcap",
			Path: "/bin/echo",
			Args: []string{
				"I am nested!",
			},
		}, garden.ProcessIO{Stdout: nestedOutput, Stderr: nestedOutput})
		Expect(err).ToNot(HaveOccurred())

		Eventually(nestedOutput, "60s").Should(gbytes.Say("I am nested!"))
	})

	Context("when cgroup limits are applied to the parent garden process", func() {

		devicesCgroupNode := func() string {
			contents, err := ioutil.ReadFile("/proc/self/cgroup")
			Expect(err).ToNot(HaveOccurred())
			for _, line := range strings.Split(string(contents), "\n") {
				if strings.Contains(line, "devices:") {
					lineParts := strings.Split(line, ":")
					Expect(lineParts).To(HaveLen(3))
					return lineParts[2]
				}
			}
			Fail("could not find devices cgroup node")
			return ""
		}

		It("passes on these limits to the child container", func() {
			// When this test is run in garden (e.g. in Concourse), we cannot create more permissive device cgroups
			// than are allowed in the outermost container. So we apply this rule to the outermost container's cgroup
			cmd := exec.Command(
				"sh",
				"-c",
				fmt.Sprintf("echo 'b 7:200 r' > /tmp/garden-%d/cgroup/devices%s/devices.allow", GinkgoParallelNode(), devicesCgroupNode()),
			)
			cmd.Stdout = GinkgoWriter
			cmd.Stderr = GinkgoWriter
			Expect(cmd.Run()).To(Succeed())

			gardenInContainer, nestedGardenAddress := startNestedGarden()
			defer client.Destroy(gardenInContainer.Handle())

			postProc, err := gardenInContainer.Run(garden.ProcessSpec{
				Path: "bash",
				User: "root",
				Args: []string{"-c",
					`
				cgroup_path_segment=$(cat /proc/self/cgroup | grep devices: | cut -d ':' -f 3)
				echo "b 7:200 r" > /tmp/garden-n/cgroup/devices${cgroup_path_segment}/devices.allow
				`},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(postProc.Wait()).To(Equal(0))

			nestedClient := gclient.New(gconn.New("tcp", nestedGardenAddress))
			nestedContainer, err := nestedClient.Create(garden.ContainerSpec{
				Privileged: true,
			})
			Expect(err).ToNot(HaveOccurred())

			nestedProcess, err := nestedContainer.Run(garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{"-c", `
				mknod ./foo b 7 200
				cat foo > /dev/null
				`},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(nestedProcess.Wait()).To(Equal(0))
		})
	})
})
