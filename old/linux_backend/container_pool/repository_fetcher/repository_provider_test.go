package repository_fetcher_test

import (
	"errors"

	"github.com/docker/docker/registry"
	"github.com/docker/docker/utils"

	. "github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RepositoryProvider", func() {
	var receivedHost string
	var receievedInsecureRegistries []string
	var receivedEndpoint *registry.Endpoint
	var endpointReturnsError error
	var sessionReturnsError error

	var returnedEndpoint *registry.Endpoint
	var returnedSession *registry.Session

	BeforeEach(func() {
		receivedHost = ""
		receievedInsecureRegistries = nil
		receivedEndpoint = nil

		endpointReturnsError = nil
		sessionReturnsError = nil

		returnedEndpoint = &registry.Endpoint{}
		RegistryNewEndpoint = func(host string, insecure []string) (*registry.Endpoint, error) {
			receivedHost = host
			receievedInsecureRegistries = insecure
			return returnedEndpoint, endpointReturnsError
		}

		returnedSession = &registry.Session{}
		RegistryNewSession = func(_ *registry.AuthConfig, _ *utils.HTTPRequestFactory, endpoint *registry.Endpoint, _ bool) (*registry.Session, error) {
			receivedEndpoint = endpoint
			return returnedSession, sessionReturnsError
		}
	})

	Context("when the hostname is empty", func() {
		It("creates a new endpoint based on the default host and port", func() {
			provider := NewRepositoryProvider("the-default-host:11", nil)
			provider.ProvideRegistry("")

			Ω(receivedHost).Should(Equal("the-default-host:11"))
		})
	})

	Context("when the hostname is not empty", func() {
		It("creates a new endpoint based on the host and port", func() {
			provider := NewRepositoryProvider("", nil)
			provider.ProvideRegistry("the-registry-host:44")

			Ω(receivedHost).Should(Equal("the-registry-host:44"))
		})
	})

	Context("when a list of secure repositories is provided", func() {
		It("creates a new endpoint passing the list of secure repositories", func() {
			provider := NewRepositoryProvider("", []string{"insecure1", "insecure2"})
			provider.ProvideRegistry("the-registry-host:44")

			Ω(receievedInsecureRegistries).Should(Equal([]string{"insecure1", "insecure2"}))
		})
	})

	Context("when NewEndpoint returns an error", func() {
		Context("and the error message does not contain `--insecure-registry`", func() {
			It("returns the error", func() {
				endpointReturnsError = errors.New("an error")
				provider := NewRepositoryProvider("", nil)

				_, err := provider.ProvideRegistry("the-registry-host:44")
				Ω(err).Should(MatchError("an error"))
			})
		})

		Context("and the error message DOES contain `--insecure-registry`", func() {
			It("returns an InsecureRegistryError", func() {
				endpointReturnsError = errors.New("some text that has --insecure-registry in it")
				provider := NewRepositoryProvider("", []string{"foo", "bar"})

				_, err := provider.ProvideRegistry("the-registry-host:44")
				Ω(err).Should(MatchError(
					&InsecureRegistryError{
						Cause:              endpointReturnsError,
						Endpoint:           "the-registry-host:44",
						InsecureRegistries: []string{"foo", "bar"},
					},
				))
			})
		})
	})

	It("creates a new session based on the endpoint", func() {
		provider := NewRepositoryProvider("", nil)
		session, err := provider.ProvideRegistry("the-registry-host:44")
		Ω(err).ShouldNot(HaveOccurred())
		Ω(session).Should(Equal(returnedSession))

		Ω(receivedEndpoint).Should(Equal(returnedEndpoint))
	})

	Context("when NewSession returns an error", func() {
		It("returns the error", func() {
			sessionReturnsError = errors.New("an error")
			provider := NewRepositoryProvider("", nil)

			_, err := provider.ProvideRegistry("the-registry-host:44")
			Ω(err).Should(MatchError("an error"))
		})
	})
})
