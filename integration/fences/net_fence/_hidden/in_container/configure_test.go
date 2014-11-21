package in_container_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Configure", func() {

	It("successfully configures the container's virtual ethernet interface", func() {
		netFencePath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/fences/mains/net-fence", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		netFenceBin := string(netFencePath)
		cmd := exec.Command(netFenceBin,
			"-target=container",
			"-containerIfcName=testPeerIfcName",
			"-containerIP=10.2.3.1",
			"-gatewayIP=10.2.3.2",
			"-subnet=10.2.3.0/30",
		)
		Ω(cmd.Run()).ShouldNot(HaveOccurred())

		out, err := exec.Command("ip", "addr").Output()
		Ω(err).ShouldNot(HaveOccurred())
		Ω(out).Should(ContainSubstring("inet 127.0.0.1/8"))
		Ω(out).Should(ContainSubstring("inet 10.2.3.1/30"))

		upcmd := exec.Command("ip", "link", "show", "lo")
		Ω(upcmd.Output()).Should(ContainSubstring("LOOPBACK,UP,")) // state is UNKNOWN !
		Ω(err).ShouldNot(HaveOccurred())

		upcmd = exec.Command("ip", "link", "show", "testPeerIfcName")
		Ω(upcmd.Output()).Should(ContainSubstring(" state UP "))
		Ω(err).ShouldNot(HaveOccurred())

		out, err = exec.Command("/bin/ping", "-c", "10", "10.2.3.2").Output()
		Ω(out).Should(ContainSubstring(" 0% packet loss"))
		Ω(err).ShouldNot(HaveOccurred())
	})
})
