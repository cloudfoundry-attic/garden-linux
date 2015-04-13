package rootfs_provider_test

import (
	"errors"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/old/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/rootfs_provider/fake_graph_driver"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/clock/fakeclock"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type FakeVolumeCreator struct {
	Created     []RootAndVolume
	CreateError error
}

type RootAndVolume struct {
	RootPath string
	Volume   string
}

func (f *FakeVolumeCreator) Create(path, v string) error {
	f.Created = append(f.Created, RootAndVolume{path, v})
	return f.CreateError
}

var _ = Describe("DockerRootFSProvider", func() {
	var (
		fakeRepositoryFetcher *fake_repository_fetcher.FakeRepositoryFetcher
		fakeGraphDriver       *fake_graph_driver.FakeGraphDriver
		fakeVolumeCreator     *FakeVolumeCreator
		fakeClock             *fakeclock.FakeClock

		provider RootFSProvider

		logger *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeRepositoryFetcher = fake_repository_fetcher.New()
		fakeGraphDriver = &fake_graph_driver.FakeGraphDriver{}
		fakeVolumeCreator = &FakeVolumeCreator{}
		fakeClock = fakeclock.NewFakeClock(time.Now())

		var err error
		provider, err = NewDocker(
			fakeRepositoryFetcher,
			fakeGraphDriver,
			fakeVolumeCreator,
			fakeClock,
		)
		Expect(err).ToNot(HaveOccurred())

		logger = lagertest.NewTestLogger("test")
	})

	Describe("ProvideRootFS", func() {
		It("fetches it and creates a graph entry with it as the parent", func() {
			fakeRepositoryFetcher.FetchResult = "some-image-id"
			fakeGraphDriver.GetReturns("/some/graph/driver/mount/point", nil)

			mountpoint, envvars, err := provider.ProvideRootFS(
				logger,
				"some-id",
				parseURL("docker:///some-repository-name"),
			)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeGraphDriver.CreateCallCount()).To(Equal(1))
			id, parent := fakeGraphDriver.CreateArgsForCall(0)
			Expect(id).To(Equal("some-id"))
			Expect(parent).To(Equal("some-image-id"))

			Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
				fake_repository_fetcher.FetchSpec{
					Repository: "docker:///some-repository-name",
					Tag:        "latest",
				},
			))

			Expect(mountpoint).To(Equal("/some/graph/driver/mount/point"))
			Expect(envvars).To(Equal(
				process.Env{
					"env1": "env1Value",
					"env2": "env2Value",
				},
			))
		})

		Context("when the image has associated VOLUMEs", func() {
			It("creates empty directories for all volumes", func() {
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				fakeGraphDriver.GetReturns("/some/graph/driver/mount/point", nil)

				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name"),
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeVolumeCreator.Created).To(Equal(
					[]RootAndVolume{
						{"/some/graph/driver/mount/point", "/foo"},
						{"/some/graph/driver/mount/point", "/bar"},
					}))
			})

			Context("when creating a volume fails", func() {
				It("returns an error", func() {
					fakeRepositoryFetcher.FetchResult = "some-image-id"
					fakeGraphDriver.GetReturns("/some/graph/driver/mount/point", nil)
					fakeVolumeCreator.CreateError = errors.New("o nooo")

					_, _, err := provider.ProvideRootFS(
						logger,
						"some-id",
						parseURL("docker:///some-repository-name"),
					)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("when the url is missing a path", func() {
			It("returns an error", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker://"),
				)
				Expect(err).To(Equal(ErrInvalidDockerURL))
			})
		})

		Context("and a tag is specified via a fragment", func() {
			It("uses it when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name#some-tag"),
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
					fake_repository_fetcher.FetchSpec{
						Repository: "docker:///some-repository-name#some-tag",
						Tag:        "some-tag",
					},
				))
			})
		})

		Context("and a host is specified", func() {
			BeforeEach(func() {
				fakeRepositoryFetcher = fake_repository_fetcher.New()
				var err error
				provider, err = NewDocker(
					fakeRepositoryFetcher,
					fakeGraphDriver,
					fakeVolumeCreator,
					fakeClock,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			It("uses the host as the registry when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker://some.host/some-repository-name"),
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
					fake_repository_fetcher.FetchSpec{
						Repository: "docker://some.host/some-repository-name",
						Tag:        "latest",
					},
				))
			})
		})

		Context("but fetching it fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeRepositoryFetcher.FetchError = disaster
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name"),
				)
				Expect(err).To(Equal(disaster))
			})
		})

		Context("but creating the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.CreateReturns(disaster)
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(logger,
					"some-id",
					parseURL("docker:///some-repository-name#some-tag"),
				)
				Expect(err).To(Equal(disaster))
			})
		})

		Context("but getting the graph entry fails", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeGraphDriver.GetReturns("", disaster)
			})

			It("returns the error", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name#some-tag"),
				)
				Expect(err).To(Equal(disaster))
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("removes the container from the rootfs graph", func() {
			err := provider.CleanupRootFS(logger, "some-id")
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeGraphDriver.PutCallCount()).To(Equal(1))
			putted := fakeGraphDriver.PutArgsForCall(0)
			Expect(putted).To(Equal("some-id"))

			Expect(fakeGraphDriver.RemoveCallCount()).To(Equal(1))
			removed := fakeGraphDriver.RemoveArgsForCall(0)
			Expect(removed).To(Equal("some-id"))
		})

		Context("when removing the container from the graph fails", func() {
			disaster := errors.New("oh no!")

			var (
				succeedsAfter int
			)

			JustBeforeEach(func() {
				retryCount := 0
				fakeGraphDriver.RemoveStub = func(id string) error {
					if retryCount > succeedsAfter {
						return nil
					}

					retryCount++
					return disaster
				}
			})

			Context("and then after a retry succeeds", func() {
				BeforeEach(func() {
					succeedsAfter = 0
				})

				It("removes the container from the rootfs graph", func() {
					done := make(chan struct{})
					go func(done chan<- struct{}) {
						err := provider.CleanupRootFS(logger, "some-id")
						Expect(err).ToNot(HaveOccurred())
						close(done)
					}(done)

					Eventually(fakeGraphDriver.RemoveCallCount).Should(Equal(1), "should not sleep before first attempt")
					fakeClock.Increment(200 * time.Millisecond)

					Eventually(fakeGraphDriver.RemoveCallCount).Should(Equal(2))
					Eventually(done).Should(BeClosed())
				})
			})

			Context("and then after many retries still fails", func() {
				BeforeEach(func() {
					succeedsAfter = 10
				})

				It("gives up and returns an error", func() {
					errs := make(chan error)
					go func(errs chan<- error) {
						errs <- provider.CleanupRootFS(logger, "some-id")
					}(errs)

					for i := 0; i < 10; i++ {
						Eventually(fakeClock.WatcherCount).Should(Equal(1))
						fakeClock.Increment(300 * time.Millisecond)
					}

					Eventually(errs).Should(Receive())
				})
			})
		})
	})
})
