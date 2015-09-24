package rootfs_provider_test

import (
	"errors"
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The Cake Co-ordinator", func() {
	var (
		fakeFetcher      *fakes.FakeRepositoryFetcher
		fakeLayerCreator *fakes.FakeLayerCreator
		fakeCake         *fake_cake.FakeCake

		cakeOrdinator *rootfs_provider.CakeOrdinator
	)

	BeforeEach(func() {
		fakeFetcher = new(fakes.FakeRepositoryFetcher)
		fakeLayerCreator = new(fakes.FakeLayerCreator)
		fakeCake = new(fake_cake.FakeCake)
		cakeOrdinator = rootfs_provider.NewCakeOrdinator(fakeCake, fakeFetcher, fakeLayerCreator)
	})

	Describe("creating container layers", func() {
		Context("When the image is succesfully fetched", func() {
			It("creates a container layer on top of the fetched layer", func() {
				image := &repository_fetcher.Image{ImageID: "my cool image"}
				fakeFetcher.FetchReturns(image, nil)
				fakeLayerCreator.CreateReturns("potato", process.Env{"foo": "bar"}, errors.New("cake"))

				rootfsPath, envs, err := cakeOrdinator.Create("container-id", &url.URL{Path: "parent"}, true, 55)
				Expect(rootfsPath).To(Equal("potato"))
				Expect(envs).To(Equal(process.Env{"foo": "bar"}))
				Expect(err).To(MatchError("cake"))

				Expect(fakeLayerCreator.CreateCallCount()).To(Equal(1))
				containerID, parentImage, translateUIDs, diskQuota := fakeLayerCreator.CreateArgsForCall(0)
				Expect(containerID).To(Equal("container-id"))
				Expect(parentImage).To(Equal(image))
				Expect(translateUIDs).To(BeTrue())
				Expect(diskQuota).To(BeEquivalentTo(55))
			})
		})

		Context("when fetching fails", func() {
			It("returns an error", func() {
				fakeFetcher.FetchReturns(nil, errors.New("amadeus"))
				_, _, err := cakeOrdinator.Create("", nil, true, 12)
				Expect(err).To(MatchError("amadeus"))
			})
		})
	})

	It("delegates removals", func() {
		fakeCake.RemoveReturns(errors.New("returned-error"))

		err := cakeOrdinator.Remove(layercake.DockerImageID("something"))
		Expect(err).To(MatchError("returned-error"))
	})

	It("prevents concurrent garbage collection and creation", func() {
		removeStarted := make(chan struct{})
		removeReturns := make(chan struct{})
		fakeCake.RemoveStub = func(id layercake.ID) error {
			close(removeStarted)
			<-removeReturns
			return nil
		}

		go cakeOrdinator.Remove(layercake.DockerImageID(""))
		<-removeStarted
		go cakeOrdinator.Create("", &url.URL{}, false, 33)

		Consistently(fakeFetcher.FetchCallCount).Should(Equal(0))
		close(removeReturns)
		Eventually(fakeFetcher.FetchCallCount).Should(Equal(1))
	})

	It("allows concurrent creation as long as deletion is not ongoing", func() {
		fakeBlocks := make(chan struct{})
		fakeFetcher.FetchStub = func(*url.URL, int64) (*repository_fetcher.Image, error) {
			<-fakeBlocks
			return nil, nil
		}

		go cakeOrdinator.Create("", &url.URL{}, false, 33)
		go cakeOrdinator.Create("", &url.URL{}, false, 33)

		Eventually(fakeFetcher.FetchCallCount).Should(Equal(2))
		close(fakeBlocks)
	})
})
