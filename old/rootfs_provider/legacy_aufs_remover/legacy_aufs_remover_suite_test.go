package legacy_aufs_remover_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestLegacyAufsRemover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LegacyAufsRemover Suite")
}
