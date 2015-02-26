package container_pool_test

import (
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/container_pool/fake_cnet"
	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Persistor", func() {
	var (
		cnPersistor container_pool.CNPersistor
		cnBuilder   cnet.Builder
		fakeCN      *fake_cnet.FakeBuilder
		cn          cnet.ContainerNetwork
		persistPath string
		ipNet       *net.IPNet
	)

	BeforeEach(func() {
		var err error
		_, ipNet, err = net.ParseCIDR("1.2.0.0/20")
		Ω(err).ShouldNot(HaveOccurred())

		fakeCN = fake_cnet.New(ipNet)
		cnBuilder = fakeCN
		cn, err = cnBuilder.Build("1.2.0.0/20", nil, "test-id")
		Ω(err).ShouldNot(HaveOccurred())
		persistPath, err = ioutil.TempDir("", "test-persistor-path")
	})

	AfterEach(func() {
		err := os.RemoveAll(persistPath)
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("a container network", func() {
		BeforeEach(func() {
			cnPersistor = container_pool.NewCNPersistor(lagertest.NewTestLogger("test"), cnBuilder)
		})
		It("can be persisted", func() {
			Ω(cnPersistor.Persist(cn, persistPath)).ShouldNot(HaveOccurred())
		})

		Context("can be recovered", func() {
			BeforeEach(func() {
				Ω(cnPersistor.Persist(cn, persistPath)).ShouldNot(HaveOccurred())
			})

			It("successfully", func() {
				recoveredCN, err := cnPersistor.Recover(persistPath)
				Ω(err).ShouldNot(HaveOccurred())

				By("and result in the same cnet content")
				Ω(recoveredCN).Should(Equal(cn))

				By("and marshal the cnet correctly")
				Ω(fakeCN.Recovered[0]).Should(Equal(`{"Subnet":"1.2.0.0/20"}`))
			})
		})

		Context("cannot be persisted", func() {
			It("when the path cannot be created", func() {
				err := cnPersistor.Persist(cn, "")
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot create persistor directory"))
			})

			It("when MarshalJSON returns an error", func() {
				fakeCN.MarshalError = errors.New("banana")
				err := cnPersistor.Persist(cn, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot marshall cnet "))
			})

			It("when the persistence file cannot be opened (for write)", func() {
				existingDir := path.Join(persistPath, "cnetConfig.json")
				err := os.MkdirAll(existingDir, 0555)
				Ω(err).ShouldNot(HaveOccurred())
				err = cnPersistor.Persist(cn, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot create persistor file "))
			})

			It("when the cnet cannot be encoded", func() {
				fakeCN.MarshalReturns = []byte{0, 0, 0, 0, 2, 0, 1}
				err := cnPersistor.Persist(cn, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot encode cnet "))
			})
		})

		Context("cannot be recovered", func() {
			BeforeEach(func() {
				Ω(cnPersistor.Persist(cn, persistPath)).ShouldNot(HaveOccurred())
			})

			It("when the persistence file cannot be opened", func() {
				_, err := cnPersistor.Recover("no-such-dir")
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot open persistor file "))
			})

			It("when the persistence file cannot be decoded", func() {
				configFile := path.Join(persistPath, "cnetConfig.json")
				err := ioutil.WriteFile(configFile, []byte{0, 1, 2}, 0755)
				Ω(err).ShouldNot(HaveOccurred())
				_, err = cnPersistor.Recover(persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot decode persistor file "))
			})

			It("when the cnet cannot be rebuilt", func() {
				fakeCN.RebuildError = errors.New("rebuild err")
				_, err := cnPersistor.Recover(persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot rebuild cnet "))
			})
		})
	})
})
