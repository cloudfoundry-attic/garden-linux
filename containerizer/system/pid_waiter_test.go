package system_test

import (
	"os/exec"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PidWaiter", func() {
	BeforeEach(func() {

	})

	FIt("waits for a process to return and returns its exit status", func() {
		cmd := exec.Command("sh", "-c", "exit 3")

		waiter := system.StartReaper()
		Expect(waiter.Start(cmd)).To(Succeed())
		Expect(waiter.Wait(cmd)).To(Equal(byte(3)))
	})
})
