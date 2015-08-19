package rootfs_provider_test

import (
	"errors"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_graph_driver"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_namespacer"
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
		fakeNamespacer        *fake_namespacer.FakeNamespacer
		fakeVolumeCreator     *FakeVolumeCreator
		fakeClock             *fakeclock.FakeClock
		name                  string

		provider RootFSProvider

		logger *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeRepositoryFetcher = fake_repository_fetcher.New()
		fakeGraphDriver = &fake_graph_driver.FakeGraphDriver{}
		fakeVolumeCreator = &FakeVolumeCreator{}
		fakeNamespacer = &fake_namespacer.FakeNamespacer{}
		fakeClock = fakeclock.NewFakeClock(time.Now())
		name = "some-name"

		var err error
		provider, err = NewDocker(
			name,
			fakeRepositoryFetcher,
			fakeGraphDriver,
			fakeVolumeCreator,
			fakeNamespacer,
			fakeClock,
		)
		Expect(err).ToNot(HaveOccurred())

		logger = lagertest.NewTestLogger("test")
	})

	Describe("Name", func() {
		It("returns correct name", func() {
			Expect(provider.Name()).To(Equal(name))
		})
	})

	Describe("ProvideRootFS", func() {
		Context("when the namespace parameter is false", func() {
			It("fetches it and creates a graph entry with it as the parent", func() {
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				fakeGraphDriver.GetReturns("/some/graph/driver/mount/point", nil)

				mountpoint, envvars, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name"),
					false,
					0,
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
		})

		Context("when the namespace parameter is true", func() {
			Context("and the image has not been translated yet", func() {
				It("fetches it, namespaces it, and creates a graph entry with it as the parent", func() {
					fakeRepositoryFetcher.FetchResult = "some-image-id"
					fakeGraphDriver.GetStub = func(id, label string) (string, error) {
						return "/mount/point/" + id, nil
					}

					fakeNamespacer.CacheKeyReturns("jam")

					mountpoint, envvars, err := provider.ProvideRootFS(
						logger,
						"some-id",
						parseURL("docker:///some-repository-name"),
						true,
						0,
					)
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
						fake_repository_fetcher.FetchSpec{
							Repository: "docker:///some-repository-name",
							Tag:        "latest",
						},
					))

					Expect(fakeGraphDriver.CreateCallCount()).To(Equal(2))
					id, parent := fakeGraphDriver.CreateArgsForCall(0)
					Expect(id).To(Equal("some-image-id@jam"))
					Expect(parent).To(Equal("some-image-id"))

					id, parent = fakeGraphDriver.CreateArgsForCall(1)
					Expect(id).To(Equal("some-id"))
					Expect(parent).To(Equal("some-image-id@jam"))

					Expect(fakeNamespacer.NamespaceCallCount()).To(Equal(1))
					dst := fakeNamespacer.NamespaceArgsForCall(0)
					Expect(dst).To(Equal("/mount/point/some-image-id@jam"))

					Expect(mountpoint).To(Equal("/mount/point/some-id"))
					Expect(envvars).To(Equal(
						process.Env{
							"env1": "env1Value",
							"env2": "env2Value",
						},
					))
				})
			})

			Context("and the image has already been translated", func() {
				BeforeEach(func() {
					fakeRepositoryFetcher.FetchResult = "some-image-id"
					fakeGraphDriver.GetStub = func(id, label string) (string, error) {
						return "/mount/point/" + id, nil
					}

					fakeNamespacer.CacheKeyReturns("sandwich")

					fakeGraphDriver.ExistsStub = func(id string) bool {
						return id == "some-image-id@sandwich"
					}
				})

				It("reuses the translated layer", func() {
					mountpoint, envvars, err := provider.ProvideRootFS(
						logger,
						"some-id",
						parseURL("docker:///some-repository-name"),
						true,
						0,
					)
					Expect(err).ToNot(HaveOccurred())

					Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
						fake_repository_fetcher.FetchSpec{
							Repository: "docker:///some-repository-name",
							Tag:        "latest",
						},
					))

					Expect(fakeGraphDriver.CreateCallCount()).To(Equal(1))
					id, parent := fakeGraphDriver.CreateArgsForCall(0)
					Expect(id).To(Equal("some-id"))
					Expect(parent).To(Equal("some-image-id@sandwich"))

					Expect(fakeNamespacer.NamespaceCallCount()).To(Equal(0))

					Expect(mountpoint).To(Equal("/mount/point/some-id"))
					Expect(envvars).To(Equal(
						process.Env{
							"env1": "env1Value",
							"env2": "env2Value",
						},
					))
				})
			})
		})

		Context("when the image has associated VOLUMEs", func() {
			It("creates empty directories for all volumes", func() {
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				fakeGraphDriver.GetReturns("/some/graph/driver/mount/point", nil)

				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name"),
					false,
					0,
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeVolumeCreator.Created).To(Equal(
					[]RootAndVolume{
						{"/some/graph/driver/mount/point", "/foo"},
						{"/some/graph/driver/mount/point", "/bar"},
					}))
			})

			Context("when a disk quota is specified", func() {
				It("should fetch with the specified disk quota", func() {
					_, _, err := provider.ProvideRootFS(
						logger,
						"some-id",
						parseURL("docker:///some-repository-name"),
						false,
						987654,
					)
					Expect(err).ToNot(HaveOccurred())

					fakeRepositoryFetcher.Fetched()
					Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
						fake_repository_fetcher.FetchSpec{
							Repository: "docker:///some-repository-name",
							Tag:        "latest",
							DiskQuota:  987654,
						},
					))
				})
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
						false,
						0,
					)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("and a tag is specified via a fragment", func() {
			It("uses it when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name#some-tag"),
					false,
					0,
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
			It("uses the host as the registry when fetching the repository", func() {
				_, _, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker://some.host/some-repository-name"),
					false,
					0,
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
					false,
					0,
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
					false,
					0,
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
					false,
					0,
				)
				Expect(err).To(Equal(disaster))
			})
		})
	})
})
