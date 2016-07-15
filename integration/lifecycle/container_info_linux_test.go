package lifecycle_test

import (
	"os"
	"syscall"

	"code.cloudfoundry.org/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container information", func() {

	BeforeEach(func() {
		client = startGarden()
	})

	Describe("for many containers", func() {
		handles := []string{"handle1", "handle2"}
		BeforeEach(func() {
			_, err := client.Create(garden.ContainerSpec{
				Handle: "handle1",
			})
			Expect(err).ToNot(HaveOccurred())
			_, err = client.Create(garden.ContainerSpec{
				Handle: "handle2",
			})
			Expect(err).ToNot(HaveOccurred())
		})

		Describe(".BulkInfo", func() {
			It("returns container info for the specified handles", func() {
				bulkInfo, err := client.BulkInfo(handles)
				Expect(err).ToNot(HaveOccurred())
				Expect(bulkInfo).To(HaveLen(2))
				for _, containerInfoEntry := range bulkInfo {
					Expect(containerInfoEntry.Err).ToNot(HaveOccurred())
				}
			})
		})

		Describe(".BulkMetrics", func() {
			BeforeEach(ensureSysfsMounted)

			It("returns container metrics for the specified handles", func() {
				bulkInfo, err := client.BulkMetrics(handles)
				Expect(err).ToNot(HaveOccurred())
				Expect(bulkInfo).To(HaveLen(2))
				for _, containerMetricsEntry := range bulkInfo {
					Expect(containerMetricsEntry.Err).ToNot(HaveOccurred())
				}
			})
		})
	})
})

func ensureSysfsMounted() {
	mntpoint, err := os.Stat("/sys")
	Expect(err).ToNot(HaveOccurred())
	parent, err := os.Stat("/")
	Expect(err).ToNot(HaveOccurred())

	if mntpoint.Sys().(*syscall.Stat_t).Dev == parent.Sys().(*syscall.Stat_t).Dev {
		Expect(syscall.Mount("sysfs", "/sys", "sysfs", uintptr(0), "")).To(Succeed())
	}
}
