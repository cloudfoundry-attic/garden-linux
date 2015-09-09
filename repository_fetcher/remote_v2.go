package repository_fetcher

import (
	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
)

//go:generate counterfeiter -o fake_remote_v2_metadata_provider/fake_remote_v2_metadata_provider.go . RemoteV2MetadataProvider
type RemoteV2MetadataProvider interface {
	ProvideMetadata(*FetchRequest) (*ImageV2Metadata, error)
}

type ImageV2Metadata struct {
	Images        []*image.Image
	ImagesDigest  []digest.Digest
	Authorization *registry.RequestAuthorization
}

type RemoteV2Fetcher struct {
	Cake             layercake.Cake
	Retainer         layercake.Retainer
	MetadataProvider RemoteV2MetadataProvider
	GraphLock        Lock
}

func (fetcher *RemoteV2Fetcher) FetchImageID(request *FetchRequest) (string, error) {
	metadata, err := fetcher.MetadataProvider.ProvideMetadata(request)
	if err != nil {
		return "", err
	}
	return metadata.Images[0].ID, nil
}

func (fetcher *RemoteV2Fetcher) Fetch(request *FetchRequest) (*FetchResponse, error) {
	metadata, err := fetcher.MetadataProvider.ProvideMetadata(request)
	if err != nil {
		return nil, err
	}

	remainingQuota := request.MaxSize

	for i := len(metadata.Images) - 1; i >= 0; i-- {
		img := metadata.Images[i]
		fetcher.Retainer.Retain(layercake.DockerImageID(img.ID))
		defer fetcher.Retainer.Release(layercake.DockerImageID(img.ID))

		var size int64
		if size, err = fetcher.fetchLayer(request, img, metadata.ImagesDigest[i], metadata.Authorization, remainingQuota); err != nil {
			return nil, err
		}

		remainingQuota = remainingQuota - size
		if remainingQuota < 0 {
			return nil, ErrQuotaExceeded
		}
	}

	return &FetchResponse{ImageID: metadata.Images[0].ID}, nil
}

func (fetcher *RemoteV2Fetcher) fetchLayer(request *FetchRequest, img *image.Image, hash digest.Digest, auth *registry.RequestAuthorization, remaining int64) (int64, error) {
	fetcher.GraphLock.Acquire(img.ID)
	defer fetcher.GraphLock.Release(img.ID)

	if img, err := fetcher.Cake.Get(layercake.DockerImageID(img.ID)); err == nil {
		return img.Size, nil
	}

	reader, _, err := request.Session.GetV2ImageBlobReader(request.Endpoint, request.RemotePath, hash, auth)
	if err != nil {
		return 0, FetchError("GetV2ImageBlobReader", request.Endpoint.URL.Host, request.Path, err)
	}
	defer reader.Close()

	err = fetcher.Cake.Register(img, &QuotaedReader{R: reader, N: remaining})
	if err != nil {
		return 0, FetchError("GraphRegister", request.Endpoint.URL.Host, request.Path, err)
	}

	return img.Size, nil
}
