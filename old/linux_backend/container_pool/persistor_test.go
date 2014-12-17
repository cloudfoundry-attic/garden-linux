package container_pool_test

import (
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/fake_fences"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Persistor", func() {
	var (
		fencePersistor container_pool.FencePersistor
		fenceBuilder   container_pool.FenceBuilder
		fakeFence      *fake_fences.FakeFences
		fence          fences.Fence
		persistPath    string
		ipNet          *net.IPNet
		ipAddr         net.IP
	)

	BeforeEach(func() {
		var err error
		ipAddr, ipNet, err = net.ParseCIDR("1.2.0.0/20")
		Ω(err).ShouldNot(HaveOccurred())

		fakeFence = fake_fences.New(ipNet)
		fenceBuilder = fakeFence
		fence, err = fenceBuilder.Build("1.2.0.0/20", nil, "test-id")
		Ω(err).ShouldNot(HaveOccurred())
		persistPath, err = ioutil.TempDir("", "test-persistor-path")
	})

	AfterEach(func() {
		err := os.RemoveAll(persistPath)
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("a (net)fence", func() {
		BeforeEach(func() {
			fencePersistor = container_pool.NewFencePersistor(lagertest.NewTestLogger("test"), fenceBuilder)
		})
		It("can be persisted", func() {
			Ω(fencePersistor.Persist(fence, persistPath)).ShouldNot(HaveOccurred())
		})

		Context("can be recovered", func() {
			BeforeEach(func() {
				Ω(fencePersistor.Persist(fence, persistPath)).ShouldNot(HaveOccurred())
			})

			It("successfully", func() {
				recoveredFence, err := fencePersistor.Recover(persistPath)
				Ω(err).ShouldNot(HaveOccurred())

				By("and result in the same fence content")
				Ω(recoveredFence).Should(Equal(fence))

				By("and marshal the fence correctly")
				Ω(fakeFence.Recovered[0]).Should(Equal(`{"Subnet":"1.2.0.0/20"}`))
			})
		})

		Context("cannot be persisted", func() {
			It("when the path cannot be created", func() {
				err := fencePersistor.Persist(fence, "")
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot create persistor directory"))
			})

			It("when MarshalJSON returns an error", func() {
				fakeFence.MarshalError = errors.New("banana")
				err := fencePersistor.Persist(fence, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot marshall fence "))
			})

			It("when the persistence file cannot be opened (for write)", func() {
				existingDir := path.Join(persistPath, "fenceConfig.json")
				err := os.MkdirAll(existingDir, 0555)
				Ω(err).ShouldNot(HaveOccurred())
				err = fencePersistor.Persist(fence, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot create persistor file "))
			})

			It("when the fence cannot be encoded", func() {
				fakeFence.MarshalReturns = []byte{0, 0, 0, 0, 2, 0, 1}
				err := fencePersistor.Persist(fence, persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot encode fence "))
			})
		})

		Context("cannot be recovered", func() {
			BeforeEach(func() {
				Ω(fencePersistor.Persist(fence, persistPath)).ShouldNot(HaveOccurred())
			})

			It("when the persistence file cannot be opened", func() {
				_, err := fencePersistor.Recover("no-such-dir")
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot open persistor file "))
			})

			It("when the persistence file cannot be decoded", func() {
				configFile := path.Join(persistPath, "fenceConfig.json")
				err := ioutil.WriteFile(configFile, []byte{0, 1, 2}, 0755)
				Ω(err).ShouldNot(HaveOccurred())
				_, err = fencePersistor.Recover(persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot decode persistor file "))
			})

			It("when the fence cannot be rebuilt", func() {
				fakeFence.RebuildError = errors.New("rebuild err")
				_, err := fencePersistor.Recover(persistPath)
				Ω(err).Should(HaveOccurred())
				Ω(err.Error()).Should(HavePrefix("Cannot rebuild fence "))
			})
		})
	})
})
