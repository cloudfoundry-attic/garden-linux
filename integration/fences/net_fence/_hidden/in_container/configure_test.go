package in_container_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	netFenceBin string
)

var _ = Describe("Configure", func() {

	BeforeEach(func() {
		netFencePath, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/fences/mains/net-fence", "-race")
		Ω(err).ShouldNot(HaveOccurred())
		netFenceBin = string(netFencePath)
	})

	It("successfully configures the container's virtual ethernet interface", func() {
		cmd := exec.Command(netFenceBin,
			"-containerIfcName=testPeerIfcName",
			"-containerIP=10.2.3.1",
			"-gatewayIP=10.2.3.2",
			"-subnet=10.2.3.0/30",
		)
		Ω(cmd.Run()).ShouldNot(HaveOccurred())
	})
})
