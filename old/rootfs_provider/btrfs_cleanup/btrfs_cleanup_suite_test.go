package btrfs_cleanup_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBtrfsCleanup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BtrfsCleanup Suite")
}
