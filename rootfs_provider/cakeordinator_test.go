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
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The Cake Co-ordinator", func() {
	var (
		fakeFetcher      *fakes.FakeRepositoryFetcher
		fakeLayerCreator *fakes.FakeLayerCreator
		fakeCake         *fake_cake.FakeCake
		logger           *lagertest.TestLogger

		cakeOrdinator *rootfs_provider.CakeOrdinator
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		fakeFetcher = new(fakes.FakeRepositoryFetcher)
		fakeLayerCreator = new(fakes.FakeLayerCreator)
		fakeCake = new(fake_cake.FakeCake)
		cakeOrdinator = rootfs_provider.NewCakeOrdinator(fakeCake, fakeFetcher, fakeLayerCreator, logger)
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

	Describe("whitelist", func() {
		It("can be set", func() {
			importantImageURL := "/my/important/image"
			anotherImportantImageURL := "/my/other/important/image"

			cakeOrdinator.WhiteList([]string{importantImageURL, anotherImportantImageURL})
			Expect(fakeFetcher.FetchIDCallCount()).To(Equal(2))

			imageURL := fakeFetcher.FetchIDArgsForCall(0)
			Expect(imageURL.String()).To(Equal(importantImageURL))

			imageURL = fakeFetcher.FetchIDArgsForCall(1)
			Expect(imageURL.String()).To(Equal(anotherImportantImageURL))
		})

		Context("when one of the URLs is invalid", func() {
			It("still can register the other valid URLs", func() {
				badURL := "ht%tq://aaaaaaaaaaaaaaaargh!"
				importantImageURL := "/my/awesome/image"

				cakeOrdinator.WhiteList([]string{badURL, importantImageURL})
				Expect(fakeFetcher.FetchIDCallCount()).To(Equal(1))

				imageURL := fakeFetcher.FetchIDArgsForCall(0)
				Expect(imageURL.String()).To(Equal(importantImageURL))
			})
		})
	})

	Describe("Remove", func() {
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

		Context("when images are whitelisted", func() {
			var (
				importantImageURL        string
				anotherImportantImageURL string
			)

			BeforeEach(func() {
				importantImageURL = "/my/important/image"
				anotherImportantImageURL = "/my/other/important/image"

				fakeFetcher.FetchIDStub = func(imageURL *url.URL) (layercake.ID, error) {
					return layercake.DockerImageID(imageURL.String()), nil
				}

				cakeOrdinator.WhiteList([]string{importantImageURL})
			})

			It("should NOT remove them", func() {
				cakeOrdinator.Remove(layercake.DockerImageID(importantImageURL))
				Expect(fakeCake.RemoveCallCount()).To(Equal(0))

			})

			It("should remove non-whitelisted image", func() {
				var imageID layercake.ID = layercake.DockerImageID(anotherImportantImageURL)
				cakeOrdinator.Remove(imageID)
				Expect(fakeCake.RemoveCallCount()).To(Equal(1))

				id := fakeCake.RemoveArgsForCall(0)
				Expect(id).To(Equal(imageID))
			})
		})
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
