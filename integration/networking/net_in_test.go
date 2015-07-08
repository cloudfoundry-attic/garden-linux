package networking_test

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gbytes"
)

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
			User: "vcap",
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
