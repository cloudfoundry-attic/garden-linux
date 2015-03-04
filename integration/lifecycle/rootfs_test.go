package lifecycle_test

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/cloudfoundry-incubator/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var dockerRegistryRootFSPath = os.Getenv("GARDEN_DOCKER_REGISTRY_TEST_ROOTFS")

var _ = Describe("Rootfs container create parameter", func() {
	var container garden.Container
	var args []string

	BeforeEach(func() {
		args = []string{}
	})

	JustBeforeEach(func() {
		client = startGarden(args...)
	})

	AfterEach(func() {
		if container != nil {
			Ω(client.Destroy(container.Handle())).Should(Succeed())
		}
	})

	Context("with a default rootfs", func() {
		It("the container is created successfully", func() {
			var err error

			container, err = client.Create(garden.ContainerSpec{RootFSPath: ""})
			Ω(err).ShouldNot(HaveOccurred())
		})
	})

	Context("with a docker rootfs URI", func() {
		Context("not containing a host", func() {
			It("the container is created successfully", func() {
				var err error

				container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker:///busybox"})
				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Context("containing a host", func() {
			Context("which is valid", func() {
				It("the container is created successfully", func() {
					var err error

					container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker://index.docker.io/busybox"})
					Ω(err).ShouldNot(HaveOccurred())
				})
			})

			Context("which is invalid", func() {
				It("the container is not created successfully", func() {
					var err error

					container, err = client.Create(garden.ContainerSpec{RootFSPath: "docker://xindex.docker.io/busybox"})
					Ω(err.Error()).Should(MatchRegexp("could not resolve"))
				})
			})

			Context("which is insecure", func() {
				var dockerRegistry garden.Container

				dockerRegistryIP := "10.0.0.1"
				dockerRegistryPort := "5001"

				if dockerRegistryRootFSPath == "" {
					log.Println("GARDEN_DOCKER_REGISTRY_TEST_ROOTFS undefined; skipping")
					return
				}

				JustBeforeEach(func() {
					dockerRegistry = startDockerRegistry(dockerRegistryIP, dockerRegistryPort)
				})

				AfterEach(func() {
					if dockerRegistry != nil {
						Ω(client.Destroy(dockerRegistry.Handle())).Should(Succeed())
					}
				})

				Context("when the host is listed in -insecureDockerRegistryList", func() {
					BeforeEach(func() {
						args = []string{
							"-insecureDockerRegistryList", dockerRegistryIP + ":" + dockerRegistryPort,
							"-allowHostAccess=true",
						}
					})

					It("creates the container successfully ", func() {
						_, err := client.Create(garden.ContainerSpec{
							RootFSPath: fmt.Sprintf("docker://%s:%s/busybox", dockerRegistryIP, dockerRegistryPort),
						})
						Ω(err).ShouldNot(HaveOccurred())
					})
				})

				Context("when the host is NOT listed in -insecureDockerRegistryList", func() {
					It("fails, and suggests the -insecureDockerRegistryList flag", func() {
						_, err := client.Create(garden.ContainerSpec{
							RootFSPath: fmt.Sprintf("docker://%s:%s/busybox", dockerRegistryIP, dockerRegistryPort),
						})

						Ω(err).Should(MatchError(ContainSubstring("-insecureDockerRegistryList")))
						Ω(err).Should(MatchError(ContainSubstring(
							"Unable to fetch RootFS image from docker://%s:%s", dockerRegistryIP, dockerRegistryPort,
						)))
					})
				})
			})
		})
	})
})

func startDockerRegistry(dockerRegistryIP string, dockerRegistryPort string) garden.Container {
	dockerRegistry, err := client.Create(
		garden.ContainerSpec{
			RootFSPath: dockerRegistryRootFSPath,
			Network:    dockerRegistryIP,
		},
	)
	Ω(err).ShouldNot(HaveOccurred())

	_, err = dockerRegistry.Run(garden.ProcessSpec{
		Env: []string{
			"DOCKER_REGISTRY_CONFIG=/docker-registry/config/config_sample.yml",
			fmt.Sprintf("REGISTRY_PORT=%s", dockerRegistryPort),
			"STANDALONE=true",
			"MIRROR_SOURCE=https://registry-1.docker.io",
			"MIRROR_SOURCE_INDEX=https://index.docker.io",
		},
		Path: "docker-registry",
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())

	Eventually(
		fmt.Sprintf("http://%s:%s/_ping", dockerRegistryIP, dockerRegistryPort),
		"5s",
	).Should(RespondToGETWith(200))

	return dockerRegistry
}

type statusMatcher struct {
	expectedStatus int

	httpError    error
	actualStatus int
}

func RespondToGETWith(expected int) types.GomegaMatcher {
	return &statusMatcher{expected, nil, 200}
}

func (m *statusMatcher) Match(actual interface{}) (success bool, err error) {
	response, err := http.Get(fmt.Sprintf("%s", actual))
	if err != nil {
		m.httpError = err
		return false, nil
	}

	m.httpError = nil
	m.actualStatus = response.StatusCode
	return response.StatusCode == m.expectedStatus, nil
}

func (m *statusMatcher) FailureMessage(actual interface{}) string {
	if m.httpError != nil {
		return fmt.Sprintf("Expected http request to have status %d but got error: %s", m.expectedStatus, m.httpError.Error())
	}

	return fmt.Sprintf("Expected http status code to be %d but was %d", m.expectedStatus, m.actualStatus)
}

func (m *statusMatcher) NegatedFailureMessage(actual interface{}) string {
	if m.httpError != nil {
		return fmt.Sprintf("Expected http request to have status %d, but got error: %s", m.expectedStatus, m.httpError.Error())
	}

	return fmt.Sprintf("Expected http status code not to be %d", m.expectedStatus)
}
