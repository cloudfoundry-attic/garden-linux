package rootfs_provider_test

import (
	"errors"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_cake"
	"github.com/cloudfoundry-incubator/garden-linux/layercake/fake_retainer"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/repository_fetcher/fake_repository_fetcher"
	. "github.com/cloudfoundry-incubator/garden-linux/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/rootfs_provider/fake_namespacer"
	"github.com/docker/docker/image"
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
		fakeCake              *fake_cake.FakeCake
		fakeRetainer          *fake_retainer.FakeRetainer
		fakeNamespacer        *fake_namespacer.FakeNamespacer
		fakeRepositoryFetcher *fake_repository_fetcher.FakeRepositoryFetcher
		fakeVolumeCreator     *FakeVolumeCreator
		fakeClock             *fakeclock.FakeClock
		name                  string

		provider RootFSProvider

		logger *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeRepositoryFetcher = fake_repository_fetcher.New()
		fakeCake = new(fake_cake.FakeCake)
		fakeRetainer = new(fake_retainer.FakeRetainer)
		fakeVolumeCreator = &FakeVolumeCreator{}
		fakeNamespacer = &fake_namespacer.FakeNamespacer{}
		fakeClock = fakeclock.NewFakeClock(time.Now())
		name = "some-name"

		var err error
		provider, err = NewDocker(
			name,
			fakeRepositoryFetcher,
			fakeCake,
			fakeRetainer,
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
		Describe("Retaining the image to avoid garbage collection", func() {
			It("releases the fetched layers", func() {
				fakeRepositoryFetcher.FetchedLayers = []string{"layer1", "layer2"}
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"), false, 0)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeRetainer.ReleaseCallCount()).To(Equal(2))
				Expect(fakeRetainer.ReleaseArgsForCall(1)).To(Equal(layercake.DockerImageID("layer1")))
				Expect(fakeRetainer.ReleaseArgsForCall(0)).To(Equal(layercake.DockerImageID("layer2")))
			})

			It("does not release the fetched image until the rootfs is created (to avoid it being garbage collected)", func() {
				fakeCake.CreateStub = func(containerID, parentImageID layercake.ID) error {
					Expect(fakeRetainer.RetainCallCount()).To(Equal(0))
					return nil
				}

				fakeRepositoryFetcher.FetchResult = "some-image-id"
				_, _, err := provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"), false, 0)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the namespace parameter is false", func() {
			It("fetches it and creates a graph entry with it as the parent", func() {
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				fakeCake.PathReturns("/some/graph/driver/mount/point", nil)

				mountpoint, envvars, err := provider.ProvideRootFS(
					logger,
					"some-id",
					parseURL("docker:///some-repository-name"),
					false,
					0,
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(fakeCake.CreateCallCount()).To(Equal(1))
				id, parent := fakeCake.CreateArgsForCall(0)
				Expect(id).To(Equal(layercake.ContainerID("some-id")))
				Expect(parent).To(Equal(layercake.DockerImageID("some-image-id")))

				Expect(fakeRepositoryFetcher.Fetched()).To(ContainElement(
					fake_repository_fetcher.FetchSpec{
						URL: parseURL("docker:///some-repository-name#latest"),
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
			It("retains the namespace layer before checking if it exists (to avoid it being garbage collected when we're going to use it)", func() {
				fakeNamespacer.CacheKeyReturns("jam")
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				fakeCake.GetStub = func(ID layercake.ID) (*image.Image, error) {
					Expect(fakeRetainer.RetainCallCount()).To(Equal(1))
					Expect(fakeRetainer.RetainArgsForCall(0)).To(Equal(layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "jam")))
					return nil, nil
				}

				provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"), true, 0)
			})

			It("releases the namespace layer, but only after the container is created", func() {
				fakeCake.CreateStub = func(id, parentID layercake.ID) error {
					Expect(fakeRetainer.ReleaseCallCount()).To(Equal(0))
					return nil
				}

				fakeNamespacer.CacheKeyReturns("jam")
				fakeRepositoryFetcher.FetchResult = "some-image-id"
				provider.ProvideRootFS(logger, "some-id", parseURL("docker:///some-repository-name"), true, 0)
				Expect(fakeRetainer.ReleaseCallCount()).To(Equal(1))
				Expect(fakeRetainer.ReleaseArgsForCall(0)).To(Equal(layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "jam")))
			})

			Context("and the image has not been translated yet", func() {
				BeforeEach(func() {
					fakeCake.GetReturns(nil, errors.New("no image here"))
				})

				It("fetches it, namespaces it, and creates a graph entry with it as the parent", func() {
					fakeRepositoryFetcher.FetchResult = "some-image-id"
					fakeCake.PathStub = func(id layercake.ID) (string, error) {
						return "/mount/point/" + id.GraphID(), nil
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
							URL: parseURL("docker:///some-repository-name#latest"),
						},
					))

					Expect(fakeCake.CreateCallCount()).To(Equal(2))
					id, parent := fakeCake.CreateArgsForCall(0)
					Expect(id).To(Equal(layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "jam")))
					Expect(parent).To(Equal(layercake.DockerImageID("some-image-id")))

					id, parent = fakeCake.CreateArgsForCall(1)
					Expect(id).To(Equal(layercake.ContainerID("some-id")))
					Expect(parent).To(Equal(layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "jam")))

					Expect(fakeNamespacer.NamespaceCallCount()).To(Equal(1))
					dst := fakeNamespacer.NamespaceArgsForCall(0)
					Expect(dst).To(Equal("/mount/point/" + layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "jam").GraphID()))

					Expect(mountpoint).To(Equal("/mount/point/" + layercake.ContainerID("some-id").GraphID()))
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
					fakeCake.PathStub = func(id layercake.ID) (string, error) {
						return "/mount/point/" + id.GraphID(), nil
					}

					fakeNamespacer.CacheKeyReturns("sandwich")

					fakeCake.GetStub = func(id layercake.ID) (*image.Image, error) {
						if id == (layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "sandwich")) {
							return &image.Image{}, nil
						}

						return nil, errors.New("hello")
					}

				})

				It("fetches the image, but reuses the translated layer", func() {
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
							URL: parseURL("docker:///some-repository-name#latest"),
						},
					))

					Expect(fakeCake.CreateCallCount()).To(Equal(1))
					id, parent := fakeCake.CreateArgsForCall(0)
					Expect(id).To(Equal(layercake.ContainerID("some-id")))
					Expect(parent).To(Equal(layercake.NamespacedID(layercake.DockerImageID("some-image-id"), "sandwich")))

					Expect(fakeNamespacer.NamespaceCallCount()).To(Equal(0))

					Expect(mountpoint).To(Equal("/mount/point/" + layercake.ContainerID("some-id").GraphID()))
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
				fakeCake.PathReturns("/some/graph/driver/mount/point", nil)

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
							URL:       parseURL("docker:///some-repository-name#latest"),
							DiskQuota: 987654,
						},
					))
				})
			})

			Context("when creating a volume fails", func() {
				It("returns an error", func() {
					fakeRepositoryFetcher.FetchResult = "some-image-id"
					fakeCake.PathReturns("/some/graph/driver/mount/point", nil)
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
						URL: parseURL("docker:///some-repository-name#some-tag"),
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
						URL: parseURL("docker://some.host/some-repository-name#latest"),
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
				fakeCake.CreateReturns(disaster)
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
				fakeCake.PathReturns("", disaster)
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
