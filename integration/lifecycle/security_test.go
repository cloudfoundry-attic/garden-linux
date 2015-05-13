package lifecycle_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Security", func() {
	Describe("Isolating PIDs", func() {
		It("isolates processes so that only process from inside the container are visible", func() {
			client = startGarden()
			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			_, err = container.Run(garden.ProcessSpec{
				Path: "sleep",
				Args: []string{"989898"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() []string {
				psout := gbytes.NewBuffer()
				ps, err := container.Run(garden.ProcessSpec{
					Path: "sh",
					Args: []string{"-c", "ps -a"},
				}, garden.ProcessIO{
					Stdout: psout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(ps.Wait()).To(Equal(0))
				return strings.Split(string(psout.Contents()), "\n")
			}).Should(HaveLen(6)) // header, wshd, sleep, sh, ps, \n
		})
	})

	Context("with a empty rootfs", func() {
		var emptyRootFSPath string

		BeforeEach(func() {
			emptyRootFSPath = os.Getenv("GARDEN_EMPTY_TEST_ROOTFS")

			if emptyRootFSPath == "" {
				Fail("GARDEN_EMPTY_TEST_ROOTFS undefined;")
			}

			client = startGarden()
		})

		It("runs a statically compiled executable in the container", func() {
			container, err := client.Create(
				garden.ContainerSpec{
					RootFSPath: emptyRootFSPath,
				},
			)
			Expect(err).ToNot(HaveOccurred())

			stdout := gbytes.NewBuffer()
			stderr := gbytes.NewBuffer()
			process, err := container.Run(
				garden.ProcessSpec{
					Path: "/hello",
				},
				garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				},
			)
			Expect(err).ToNot(HaveOccurred())

			exitStatus, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitStatus).To(Equal(0))

			Expect(string(stdout.Contents())).To(Equal("hello from stdout"))
			Expect(string(stderr.Contents())).To(Equal("hello from stderr"))
		})
	})

	Describe("Denying access to network ranges", func() {
		var (
			blockedListener   garden.Container
			blockedListenerIP string = fmt.Sprintf("11.0.%d.1", GinkgoParallelNode())

			unblockedListener   garden.Container
			unblockedListenerIP string = fmt.Sprintf("11.1.%d.1", GinkgoParallelNode())

			allowedListener   garden.Container
			allowedListenerIP string = fmt.Sprintf("11.2.%d.1", GinkgoParallelNode())

			sender garden.Container
		)

		BeforeEach(func() {
			client = startGarden(
				"-denyNetworks", strings.Join([]string{
					blockedListenerIP + "/32",
					allowedListenerIP + "/32",
				}, ","),
				"-allowNetworks", allowedListenerIP+"/32",
			)

			var err error

			// create a listener to which we deny network access
			blockedListener, err = client.Create(garden.ContainerSpec{Network: blockedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			blockedListenerIP = containerIP(blockedListener)

			// create a listener to which we do not deny access
			unblockedListener, err = client.Create(garden.ContainerSpec{Network: unblockedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			unblockedListenerIP = containerIP(unblockedListener)

			// create a listener to which we exclicitly allow access
			allowedListener, err = client.Create(garden.ContainerSpec{Network: allowedListenerIP + "/30"})
			Expect(err).ToNot(HaveOccurred())
			allowedListenerIP = containerIP(allowedListener)

			// create a container with the new deny network configuration
			sender, err = client.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

		})

		AfterEach(func() {
			err := client.Destroy(sender.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(blockedListener.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(unblockedListener.Handle())
			Expect(err).ToNot(HaveOccurred())

			err = client.Destroy(allowedListener.Handle())
			Expect(err).ToNot(HaveOccurred())
		})

		runInContainer := func(container garden.Container, script string) garden.Process {
			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", script},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			return process
		}

		It("makes that block of ip addresses inaccessible to the container", func() {
			runInContainer(blockedListener, "nc -l 0.0.0.0:12345")
			runInContainer(unblockedListener, "nc -l 0.0.0.0:12345")
			runInContainer(allowedListener, "nc -l 0.0.0.0:12345")

			// a bit of time for the listeners to start, since they block
			time.Sleep(time.Second)

			process := runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP),
			)
			Expect(process.Wait()).To(Equal(1))

			process = runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", unblockedListenerIP),
			)
			Expect(process.Wait()).To(Equal(0))

			process = runInContainer(
				sender,
				fmt.Sprintf("echo hello | nc -w 1 %s 12345", allowedListenerIP),
			)
			Expect(process.Wait()).To(Equal(0))
		})
	})
})
