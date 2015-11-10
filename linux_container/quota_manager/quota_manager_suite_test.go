package quota_manager_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestQuotaManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "QuotaManager Suite")
}
