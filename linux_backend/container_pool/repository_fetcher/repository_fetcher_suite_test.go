package repository_fetcher_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestRepositoryFetcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RepositoryFetcher Suite")
}
