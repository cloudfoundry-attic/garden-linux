package main_test

import (
	"fmt"
	"os/exec"

	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("capcheck", func() {
	BeforeEach(func() {
		if os.Getuid() != 0 {
			Skip("must be run as root")
		}
	})

	describeCapability := func(cap string, expectedError string) {
		Describe("probe "+cap, func() {
			Context("when a process does have "+cap, func() { // assumes tests are run as root
				It("succeeds", func() {
					session, err := gexec.Start(exec.Command("capsh", "--inh="+cap, "--", "-c", fmt.Sprintf("%s %s", capabilityTestBin, cap)), GinkgoWriter, GinkgoWriter)
					Expect(err).NotTo(HaveOccurred())
					Eventually(session).Should(gexec.Exit(0))
				})
			})

			Context("when a process does not have "+cap, func() {
				It("logs an error and returns a bad exit status code", func() {
					session, err := gexec.Start(exec.Command("capsh", "--inh=", "--drop="+cap, "--", "-c", fmt.Sprintf("%s %s", capabilityTestBin, cap)), GinkgoWriter, GinkgoWriter)
					Expect(err).NotTo(HaveOccurred())
					Eventually(session).Should(gbytes.Say(expectedError))
					Eventually(session).Should(gexec.Exit(1))
				})
			})
		})
	}

	caps := []struct {
		Cap           string
		ExpectedError string
	}{
		{"CAP_MKNOD", "Operation not permitted"},
		{"CAP_NET_BIND_SERVICE", "Failed to create listener: listen tcp :21: bind: permission denied"},
		{"CAP_SYS_ADMIN", "Failed to create a bind mount: operation not permitted"},
	}

	for _, cap := range caps {
		describeCapability(cap.Cap, cap.ExpectedError)
	}
})
