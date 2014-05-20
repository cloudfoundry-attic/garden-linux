package rootfs_provider_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestRootfsProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RootfsProvider Suite")
}
