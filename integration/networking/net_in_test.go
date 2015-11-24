package networking_test

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/integration/runner"
	"github.com/onsi/gomega/gbytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func restartGarden(argv ...string) *runner.RunningGarden {
	Expect(client.Ping()).To(Succeed(), "tried to restart garden while it was not running")
	Expect(client.Stop()).To(Succeed())
	return startGarden(argv...)
}

var _ = Describe("Net In", func() {
	var (
		container     garden.Container
		containerPort uint32
		hostPort      uint32
		externalIP    string
	)

	const containerHandle = "6e4ea858-6b31-4243-5dcc-093cfb83952d"

	var listenInContainer = func(container garden.Container, containerPort uint32) error {
		_, err := container.Run(garden.ProcessSpec{
			User: "alice",
			Path: "sh",
			Args: []string{"-c", fmt.Sprintf("echo %d | nc -l %d", containerPort, containerPort)},
		}, garden.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})
		Expect(err).ToNot(HaveOccurred())
		time.Sleep(2 * time.Second)

		return err
	}

	var sendRequest = func(ip string, port uint32) *gbytes.Buffer {
		stdout := gbytes.NewBuffer()
		cmd := exec.Command("nc", "-w1", ip, fmt.Sprintf("%d", port))
		cmd.Stdout = stdout
		cmd.Stderr = GinkgoWriter

		err := cmd.Run()
		Expect(err).ToNot(HaveOccurred())

		return stdout
	}

	BeforeEach(func() {
		client = startGarden()

		var err error
		container, err = client.Create(garden.ContainerSpec{
			RootFSPath: "/opt/warden/nestable-rootfs",
			Privileged: true,
		})
		Expect(err).ToNot(HaveOccurred())

		info, err := container.Info()
		Expect(err).ToNot(HaveOccurred())
		externalIP = info.ExternalIP
		hostPort = 8888
		containerPort = 8080
	})

	AfterEach(func() {
		err := client.Destroy(container.Handle())
		Expect(err).ToNot(HaveOccurred())
	})

	It("maps the provided host port to the container port", func() {
		_, _, err := container.NetIn(hostPort, containerPort)
		Expect(err).ToNot(HaveOccurred())

		Expect(listenInContainer(container, containerPort)).To(Succeed())

		stdout := sendRequest(externalIP, hostPort)
		Expect(stdout).To(gbytes.Say(fmt.Sprintf("%d", containerPort)))
	})

	Context("when multiple netin calls map the same host port to distinct container ports", func() {
		var containerPort2 uint32

		BeforeEach(func() {
			containerPort2 = uint32(8081)

			_, _, err := container.NetIn(hostPort, containerPort)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(hostPort, containerPort2)
			Expect(err).ToNot(HaveOccurred())
		})

		// The following behaviour is a bug. See #98463890.
		It("routes the request to the first container port", func() {
			Expect(listenInContainer(container, containerPort)).To(Succeed())
			Expect(listenInContainer(container, containerPort2)).To(Succeed())

			stdout := sendRequest(externalIP, hostPort)
			Expect(stdout).To(gbytes.Say(fmt.Sprintf("%d", containerPort)))
		})
	})

	Context("when multiple netin calls map distinct host ports to the same container port", func() {
		var hostPort2 uint32

		BeforeEach(func() {
			hostPort2 = uint32(8889)

			_, _, err := container.NetIn(hostPort, containerPort)
			Expect(err).ToNot(HaveOccurred())

			_, _, err = container.NetIn(hostPort2, containerPort)
			Expect(err).ToNot(HaveOccurred())
		})

		It("routes request from either host port to the container port", func() {
			Expect(listenInContainer(container, containerPort)).To(Succeed())
			stdout := sendRequest(externalIP, hostPort)
			Expect(stdout).To(gbytes.Say(fmt.Sprintf("%d", containerPort)))

			Expect(listenInContainer(container, containerPort)).To(Succeed())
			stdout = sendRequest(externalIP, hostPort2)
			Expect(stdout).To(gbytes.Say(fmt.Sprintf("%d", containerPort)))
		})
	})
})

var _ = Describe("Port Selection", func() {
	var (
		portPoolSize int
		extraArgs    []string
	)

	BeforeEach(func() {
		portPoolSize = 100
		extraArgs = []string{}
	})

	JustBeforeEach(func() {
		args := append(
			[]string{"--portPoolSize", fmt.Sprintf("%d", portPoolSize)},
			extraArgs...,
		)
		client = startGarden(args...)
	})

	It("should not reuse ports of destroyed containers", func() {
		container, err := client.Create(garden.ContainerSpec{})
		Expect(err).NotTo(HaveOccurred())

		oldHostPort, _, err := container.NetIn(0, 0)
		Expect(err).NotTo(HaveOccurred())

		client.Destroy(container.Handle())

		container, err = client.Create(garden.ContainerSpec{})
		Expect(err).NotTo(HaveOccurred())

		newHostPort, _, err := container.NetIn(0, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(newHostPort).To(BeNumerically("==", oldHostPort+1))
	})

	Context("when server is restarted", func() {
		It("should not reuse ports", func() {
			var (
				containers   []string
				lastHostPort uint32
			)

			for index := 0; index < 2; index++ {
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				hostPort, _, err := container.NetIn(0, 0)
				Expect(err).NotTo(HaveOccurred())

				containers = append(containers, container.Handle())
				lastHostPort = hostPort
			}

			for index := 0; index < 2; index++ {
				Expect(client.Destroy(containers[index])).To(Succeed())
			}

			client = restartGarden("--portPoolSize", fmt.Sprintf("%d", portPoolSize))

			container, err := client.Create(garden.ContainerSpec{})
			Expect(err).NotTo(HaveOccurred())

			newHostPort, _, err := container.NetIn(0, 0)
			Expect(err).NotTo(HaveOccurred())

			Expect(newHostPort).To(BeNumerically("==", lastHostPort+1))
		})

		Context("and the port range is reduced", func() {
			It("should start from the first port", func() {
				var (
					containers    []string
					firstHostPort uint32
				)

				for index := 0; index < 2; index++ {
					container, err := client.Create(garden.ContainerSpec{})
					Expect(err).NotTo(HaveOccurred())

					hostPort, _, err := container.NetIn(0, 0)
					Expect(err).NotTo(HaveOccurred())

					containers = append(containers, container.Handle())
					if index == 0 {
						firstHostPort = hostPort
					}
				}

				for index := 0; index < 2; index++ {
					Expect(client.Destroy(containers[index])).To(Succeed())
				}

				client = restartGarden("--portPoolSize", fmt.Sprintf("%d", 1))

				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				newHostPort, _, err := container.NetIn(0, 0)
				Expect(err).NotTo(HaveOccurred())

				Expect(newHostPort).To(BeNumerically("==", firstHostPort))
			})
		})

		Context("and the port range is exhausted with snapshotting disabled", func() {
			BeforeEach(func() {
				portPoolSize = 1
				extraArgs = append(extraArgs, "--snapshots", "")
			})

			It("returns the first port in the range", func() {
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				firstHostPort, _, err := container.NetIn(0, 0)
				Expect(err).NotTo(HaveOccurred())

				client = restartGarden("--portPoolSize", fmt.Sprintf("%d", 2))

				container, err = client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				newHostPort, _, err := container.NetIn(0, 0)
				Expect(err).NotTo(HaveOccurred())

				Expect(newHostPort).To(BeNumerically("==", firstHostPort))
			})
		})

		Context("and the port range is exhausted with snapshotting enabled", func() {
			BeforeEach(func() {
				portPoolSize = 1
				snapshotsPath, err := ioutil.TempDir("", "snapshots")
				Expect(err).NotTo(HaveOccurred())
				extraArgs = append(extraArgs, "--snapshots", snapshotsPath)
			})

			It("stays exhausted after a restart", func() {
				container, err := client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				_, _, err = container.NetIn(0, 0)
				Expect(err).NotTo(HaveOccurred())

				args := append(extraArgs, "--portPoolSize", fmt.Sprintf("%d", portPoolSize))
				client = restartGarden(args...)

				containers, err := client.Containers(garden.Properties{})
				Expect(err).NotTo(HaveOccurred())
				Expect(containers).To(HaveLen(1))

				container, err = client.Create(garden.ContainerSpec{})
				Expect(err).NotTo(HaveOccurred())

				_, _, err = container.NetIn(0, 0)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
