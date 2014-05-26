package rootfs_provider_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net/url"

	"testing"
)

func parseURL(str string) *url.URL {
	parsedURL, err := url.Parse(str)
	Expect(err).ToNot(HaveOccurred())

	return parsedURL
}

func TestRootfsProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RootfsProvider Suite")
}
