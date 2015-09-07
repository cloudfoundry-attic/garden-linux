package repository_fetcher

import (
	"encoding/json"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type imageV2Metadata struct {
	Images        []*image.Image
	ImagesDigest  []digest.Digest
	Authorization *registry.RequestAuthorization
}

type RemoteV2Fetcher struct {
	Cake      layercake.Cake
	Retainer  layercake.Retainer
	GraphLock Lock
}

func (fetcher *RemoteV2Fetcher) FetchImageID(request *FetchRequest) (string, error) {
	metadata, err := fetcher.fetchMetadata(request)
	if err != nil {
		return "", err
	}
	return metadata.Images[0].ID, nil
}

func (fetcher *RemoteV2Fetcher) Fetch(request *FetchRequest) (*FetchResponse, error) {
	metadata, err := fetcher.fetchMetadata(request)
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

func (fetcher *RemoteV2Fetcher) fetchMetadata(request *FetchRequest) (*imageV2Metadata, error) {
	request.Logger.Debug("docker-v2-fetch", lager.Data{
		"request": request,
	})

	auth, err := request.Session.GetV2Authorization(request.Endpoint, request.RemotePath, true)
	if err != nil {
		return nil, FetchError("GetV2Authorization", request.Endpoint.URL.Host, request.Path, err)
	}

	_, manifestBytes, err := request.Session.GetV2ImageManifest(request.Endpoint, request.RemotePath, request.Tag, auth)
	if err != nil {
		return nil, FetchError("GetV2ImageManifest", request.Endpoint.URL.Host, request.Path, err)
	}

	var manifest registry.ManifestData
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, FetchError("UnmarshalManifest", request.Endpoint.URL.Host, request.Path, err)
	}

	var hashes []digest.Digest
	var images []*image.Image

	for index, layer := range manifest.FSLayers {
		hash, err := digest.ParseDigest(layer.BlobSum)
		if err != nil {
			return nil, FetchError("ParseDigest", request.Endpoint.URL.Host, request.Path, err)
		}

		img, err := image.NewImgJSON([]byte(manifest.History[index].V1Compatibility))
		if err != nil {
			return nil, FetchError("NewImgJSON", request.Endpoint.URL.Host, request.Path, err)
		}

		images = append(images, img)
		hashes = append(hashes, hash)
	}

	return &imageV2Metadata{
		Images:        images,
		ImagesDigest:  hashes,
		Authorization: auth}, nil
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
