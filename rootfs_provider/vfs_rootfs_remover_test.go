package rootfs_provider_test

import (
	"errors"

	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_graph_driver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("VfsRootfsRemover", func() {
	var fakeGraphDriver *fake_graph_driver.FakeGraphDriver
	var vsfRemover *rootfs_provider.VfsRootFSRemover
	var logger *lagertest.TestLogger

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("VfsRootfsRemover")
		fakeGraphDriver = new(fake_graph_driver.FakeGraphDriver)
		vsfRemover = &rootfs_provider.VfsRootFSRemover{
			GraphDriver: fakeGraphDriver,
		}
	})

	It("removes the container from the rootfs graph", func() {
		err := vsfRemover.CleanupRootFS(logger, "some-id")
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeGraphDriver.PutCallCount()).To(Equal(1))
		putted := fakeGraphDriver.PutArgsForCall(0)
		Expect(putted).To(Equal("some-id"))

		Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(1))
		removed := fakeGraphDriver.RemoveArgsForCall(0)
		Expect(removed).To(Equal("some-id"))
	})

	Context("when removing the container from the graph fails", func() {
		JustBeforeEach(func() {
			fakeGraphDriver.RemoveReturns(errors.New("oh no!"))
		})

		It("returns the error", func() {
			Expect(vsfRemover.CleanupRootFS(logger, "oi")).To(MatchError("oh no!"))
		})
	})
})
