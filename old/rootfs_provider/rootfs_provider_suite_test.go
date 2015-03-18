package rootfs_provider_test

import (
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func parseURL(str string) *url.URL {
	parsedURL, err := url.Parse(str)
	Ω(err).ShouldNot(HaveOccurred())

	return parsedURL
}

func TestRootfsProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RootfsProvider Suite")
}
