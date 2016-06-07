package healthchecker_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/healthchecker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NetworkAllocationHealthChecker", func() {

	Context("when a conflict has been detected", func() {
		It("returns an error", func() {
			hc := &healthchecker.NetworkAllocationHealthChecker{}
			err := errors.New("conflict detected during IP allocation")
			hc.ConflictDetected(err)
			Expect(hc.HealthCheck()).To(MatchError(err.Error()))
		})
	})

	Context("when no conflict has been detected", func() {
		It("does not return an error", func() {
			hc := &healthchecker.NetworkAllocationHealthChecker{}
			Expect(hc.HealthCheck()).To(Succeed())
		})
	})
})
