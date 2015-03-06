package rootfs_provider_test

import (
	"errors"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/rootfs_provider/fake_graph_driver"
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
		Ω(err).ShouldNot(HaveOccurred())

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
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeGraphDriver.CreateCallCount()).Should(Equal(1))
			id, parent := fakeGraphDriver.CreateArgsForCall(0)
			Ω(id).Should(Equal("some-id"))
			Ω(parent).Should(Equal("some-image-id"))

			Ω(fakeRepositoryFetcher.Fetched()).Should(ContainElement(
				fake_repository_fetcher.FetchSpec{
					Repository: "docker:///some-repository-name",
					Tag:        "latest",
				},
			))

			Ω(mountpoint).Should(Equal("/some/graph/driver/mount/point"))
			Ω(envvars).Should(Equal(
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
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeVolumeCreator.Created).Should(Equal(
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
					Ω(err).Should(HaveOccurred())
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
				Ω(err).Should(Equal(ErrInvalidDockerURL))
			})
		})

		Context("and a tag is specified via a fragment", func() {
			It("uses it when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name#some-tag"),
				)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRepositoryFetcher.Fetched()).Should(ContainElement(
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
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("uses the host as the registry when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker://some.host/some-repository-name"),
				)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(fakeRepositoryFetcher.Fetched()).Should(ContainElement(
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
				Ω(err).Should(Equal(disaster))
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
				Ω(err).Should(Equal(disaster))
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
				Ω(err).Should(Equal(disaster))
			})
		})
	})

	Describe("CleanupRootFS", func() {
		It("removes the container from the rootfs graph", func() {
			err := provider.CleanupRootFS(logger, "some-id")
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fakeGraphDriver.PutCallCount()).Should(Equal(1))
			putted := fakeGraphDriver.PutArgsForCall(0)
			Ω(putted).Should(Equal("some-id"))

			Ω(fakeGraphDriver.RemoveCallCount()).Should(Equal(1))
			removed := fakeGraphDriver.RemoveArgsForCall(0)
			Ω(removed).Should(Equal("some-id"))
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
						Ω(err).ShouldNot(HaveOccurred())
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
