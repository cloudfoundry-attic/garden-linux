package repository_fetcher_test

import (
	"errors"
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("The Cake Co-ordinator", func() {
	var (
		fakeFetcher   *fakes.FakeRepositoryFetcher
		fakeCake      *fake_cake.FakeCake
		cakeOrdinator *repository_fetcher.CakeOrdinator
	)

	BeforeEach(func() {
		fakeFetcher = new(fakes.FakeRepositoryFetcher)
		fakeCake = new(fake_cake.FakeCake)
		cakeOrdinator = repository_fetcher.NewCakeOrdinator(fakeCake, fakeFetcher)
	})

	It("delegates fetches", func() {
		remoteImage := &repository_fetcher.Image{ImageID: "my cool image"}
		fakeFetcher.FetchReturns(remoteImage, errors.New("potato"))

		image, err := cakeOrdinator.Fetch(&url.URL{Path: "blah"}, 0)
		Expect(err).To(MatchError("potato"))
		Expect(image).To(Equal(remoteImage))
	})

	It("delegates removals", func() {
		fakeCake.RemoveReturns(errors.New("returned-error"))

		err := cakeOrdinator.Remove(layercake.DockerImageID("something"))
		Expect(err).To(MatchError("returned-error"))
	})

	It("prevents concurrent garbage collection and fetching", func() {
		removeStarted := make(chan struct{})
		removeReturns := make(chan struct{})
		fakeCake.RemoveStub = func(id layercake.ID) error {
			close(removeStarted)
			<-removeReturns
			return nil
		}

		go cakeOrdinator.Remove(layercake.DockerImageID(""))
		<-removeStarted
		go cakeOrdinator.Fetch(&url.URL{}, 33)

		Consistently(fakeFetcher.FetchCallCount).Should(Equal(0))
		close(removeReturns)
		Eventually(fakeFetcher.FetchCallCount).Should(Equal(1))
	})

	It("allows concurrent fetching as long as deletion is not ongoing", func() {
		fakeBlocks := make(chan struct{})
		fakeFetcher.FetchStub = func(*url.URL, int64) (*repository_fetcher.Image, error) {
			<-fakeBlocks
			return nil, nil
		}

		go cakeOrdinator.Fetch(&url.URL{}, 33)
		go cakeOrdinator.Fetch(&url.URL{}, 43)

		Eventually(fakeFetcher.FetchCallCount).Should(Equal(2))
		close(fakeBlocks)
	})
})
