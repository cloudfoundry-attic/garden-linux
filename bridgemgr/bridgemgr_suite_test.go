package bridgemgr_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBridgeManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bridge Manager Suite")
}
