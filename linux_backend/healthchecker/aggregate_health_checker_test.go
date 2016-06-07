package healthchecker_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/healthchecker"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/healthchecker/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AggregateHealthChecker", func() {
	var (
		ahc     *healthchecker.AggregateHealthChecker
		fakeOne *fakes.FakeHealthChecker
		fakeTwo *fakes.FakeHealthChecker
	)

	BeforeEach(func() {
		fakeOne = new(fakes.FakeHealthChecker)
		fakeTwo = new(fakes.FakeHealthChecker)

		ahc = &healthchecker.AggregateHealthChecker{
			HealthCheckers: []healthchecker.HealthChecker{
				fakeOne, fakeTwo,
			},
		}
	})

	It("should iterate over a list of healthcheckers", func() {
		ahc.HealthCheck()
		Expect(fakeOne.HealthCheckCallCount()).To(Equal(1))
		Expect(fakeTwo.HealthCheckCallCount()).To(Equal(1))
	})

	Context("when a health check fails", func() {
		BeforeEach(func() {
			fakeTwo.HealthCheckReturns(errors.New("health-check-failed"))
		})

		It("should return the error", func() {
			Expect(ahc.HealthCheck()).To(MatchError("health-check-failed"))
		})

		Context("when multiple health checks fail", func() {
			BeforeEach(func() {
				fakeOne.HealthCheckReturns(errors.New("first-health-check-failed"))
				fakeTwo.HealthCheckReturns(errors.New("health-check-failed"))
			})

			It("should return the first error", func() {
				Expect(ahc.HealthCheck()).To(MatchError("first-health-check-failed"))
				Expect(fakeTwo.HealthCheckCallCount()).To(Equal(0))
			})
		})
	})

})
