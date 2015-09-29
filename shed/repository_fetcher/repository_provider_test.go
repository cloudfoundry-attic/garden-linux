package repository_fetcher_test

import (
	"errors"
	"net/http"

	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/registry"

	. "github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RepositoryProvider", func() {
	var receivedIndexName string
	var receivedIndexSecure bool

	var receivedEndpoint *registry.Endpoint
	var receivedAuthConfig *cliconfig.AuthConfig
	var endpointReturnsError error
	var sessionReturnsError error

	var returnedEndpoint *registry.Endpoint
	var returnedSession *registry.Session

	BeforeEach(func() {
		receivedIndexName = ""
		receivedIndexSecure = false
		receivedEndpoint = nil
		receivedAuthConfig = nil

		endpointReturnsError = nil
		sessionReturnsError = nil

		returnedEndpoint = &registry.Endpoint{}
		RegistryNewEndpoint = func(indexInfo *registry.IndexInfo, header http.Header) (*registry.Endpoint, error) {
			receivedIndexName = indexInfo.Name
			receivedIndexSecure = indexInfo.Secure
			return returnedEndpoint, endpointReturnsError
		}

		returnedSession = &registry.Session{}
		RegistryNewSession = func(client *http.Client, authConfig *cliconfig.AuthConfig, endpoint *registry.Endpoint) (*registry.Session, error) {
			receivedEndpoint = endpoint
			receivedAuthConfig = authConfig
			return returnedSession, sessionReturnsError
		}
	})

	Context("when the hostname is empty", func() {
		It("creates a new endpoint based on the default host and port", func() {
			provider := NewRepositoryProvider("the-default-host:11", nil)
			provider.ProvideRegistry("")

			Expect(receivedIndexName).To(Equal("the-default-host:11"))
			Expect(receivedIndexSecure).To(Equal(true))
		})
	})

	Context("when the hostname is not empty", func() {
		It("creates a new endpoint based on the supplied host and port", func() {
			provider := NewRepositoryProvider("", nil)
			provider.ProvideRegistry("the-registry-host:44")

			Expect(receivedIndexName).To(Equal("the-registry-host:44"))
		})
	})

	Context("when a list of secure repositories is provided", func() {
		Context("and the requested endpoint is in the list", func() {
			It("returns that the registry is insecure", func() {
				provider := NewRepositoryProvider("", []string{"insecure1", "insecure2"})
				provider.ProvideRegistry("insecure1")

				Expect(receivedIndexSecure).To(Equal(false))
			})

			Context("and the list is using an IP", func() {
				It("returns that the registry is insecure", func() {
					provider := NewRepositoryProvider("", []string{"100.100.100.0/24", "103.100.100.15"})
					provider.ProvideRegistry("103.100.100.15")

					Expect(receivedIndexSecure).To(Equal(false))
				})
			})

			Context("and the list is using CIDR addresses", func() {
				It("returns that the registry is insecure", func() {
					provider := NewRepositoryProvider("", []string{"100.100.100.0/24", "103.100.100.0/24"})
					provider.ProvideRegistry("100.100.100.155")

					Expect(receivedIndexSecure).To(Equal(false))
				})
			})

			Context("and the list is using CIDR addresses and hostnames", func() {
				It("returns that the registry is insecure", func() {
					provider := NewRepositoryProvider("", []string{"100.100.100.0/24", "103.100.100.0/24", "hostname1"})
					provider.ProvideRegistry("hostname1")

					Expect(receivedIndexSecure).To(Equal(false))
				})
			})
		})

		Context("and the requested endpoint is not in the list", func() {
			It("assumes the registry is secure", func() {
				provider := NewRepositoryProvider("", []string{"insecure1", "insecure2"})
				provider.ProvideRegistry("the-registry-host:44")

				Expect(receivedIndexSecure).To(Equal(true))
			})

			Context("and the list is using CIDR addresses", func() {
				It("returns that the registry is secure", func() {
					provider := NewRepositoryProvider("", []string{"100.100.100.0/24", "103.100.100.0/24"})
					provider.ProvideRegistry("100.100.95.155")

					Expect(receivedIndexSecure).To(Equal(true))
				})
			})
		})
	})

	Context("when NewEndpoint returns an error", func() {
		Context("and the error message does not contain `--insecure-registry`", func() {
			It("returns the error", func() {
				endpointReturnsError = errors.New("an error")
				provider := NewRepositoryProvider("", nil)

				_, _, err := provider.ProvideRegistry("the-registry-host:44")
				Expect(err).To(MatchError("an error"))
			})
		})

		Context("and the error message DOES contain `--insecure-registry`", func() {
			It("returns an InsecureRegistryError", func() {
				endpointReturnsError = errors.New("some text that has --insecure-registry in it")
				provider := NewRepositoryProvider("", []string{"foo", "bar"})

				_, _, err := provider.ProvideRegistry("the-registry-host:44")
				Expect(err).To(MatchError(
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
		session, _, err := provider.ProvideRegistry("the-registry-host:44")
		Expect(err).ToNot(HaveOccurred())
		Expect(session).To(Equal(returnedSession))

		Expect(receivedEndpoint).To(Equal(returnedEndpoint))
		Expect(receivedAuthConfig).ToNot(BeNil())
	})

	Context("when NewSession returns an error", func() {
		It("returns the error", func() {
			sessionReturnsError = errors.New("an error")
			provider := NewRepositoryProvider("", nil)

			_, _, err := provider.ProvideRegistry("the-registry-host:44")
			Expect(err).To(MatchError("an error"))
		})
	})
})
