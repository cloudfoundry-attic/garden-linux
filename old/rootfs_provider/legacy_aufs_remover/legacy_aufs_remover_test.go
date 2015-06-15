package legacy_aufs_remover_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/legacy_aufs_remover"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/legacy_aufs_remover/fake_unmounter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("LegacyAufsRemover", func() {
	var (
		aufsRemover *legacy_aufs_remover.Remover
		overlaysDir = "/some/overlays/dir"
		logger      *lagertest.TestLogger
		unmounter   *fake_unmounter.FakeUnmounter
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("legacy-aufs-remover-test")

		unmounter = new(fake_unmounter.FakeUnmounter)

		depot, err := ioutil.TempDir("", "depot")
		Expect(err).ToNot(HaveOccurred())
		Expect(err).ToNot(HaveOccurred())

		aufsRemover = &legacy_aufs_remover.Remover{
			DepotDir:  depot,
			Unmounter: unmounter,
		}

		etcDir := filepath.Join(depot, "container-1", "etc")
		Expect(os.MkdirAll(etcDir, 0755)).To(Succeed())
		Expect(ioutil.WriteFile(filepath.Join(etcDir, "config"), []byte(fmt.Sprintf(`id=pvaoftuk0pf
network_host_ip=10.254.0.2
network_host_iface=wpvaoftuk0pf-0
network_container_ip=10.254.0.1
network_container_iface=wpvaoftuk0pf-1
bridge_iface=wb-pvaoftubqrk0
network_cidr_suffix=30
container_iface_mtu=1500
network_cidr=10.254.0.0/30
root_uid=600000
user_uid=610001
rootfs_path=%s
external_ip=10.0.2.15
`, overlaysDir)), 0644)).To(Succeed())
	})

	It("removes the overlays directory", func() {
		Expect(aufsRemover.CleanupRootFS(logger, "container-1")).To(Succeed())
		Expect(unmounter.UnmountCallCount()).To(Equal(1))
		Expect(unmounter.UnmountArgsForCall(0)).To(Equal(overlaysDir))
	})
})
