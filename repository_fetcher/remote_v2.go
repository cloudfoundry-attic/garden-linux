package repository_fetcher

import (
	"encoding/json"

	"github.com/docker/distribution/digest"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type RemoteV2Fetcher struct {
	Graph     Graph
	GraphLock Lock
}

func (fetcher *RemoteV2Fetcher) Fetch(request *FetchRequest) (*FetchResponse, error) {
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

	var lastImg *image.Image

	for i := len(manifest.FSLayers) - 1; i >= 0; i-- {
		hash, err := digest.ParseDigest(manifest.FSLayers[i].BlobSum)
		if err != nil {
			return nil, FetchError("ParseDigest", request.Endpoint.URL.Host, request.Path, err)
		}

		img, err := image.NewImgJSON([]byte(manifest.History[i].V1Compatibility))
		if err != nil {
			return nil, FetchError("NewImgJSON", request.Endpoint.URL.Host, request.Path, err)
		}
		if i == 0 {
			lastImg = img
		}

		if err := fetcher.fetchLayer(request, img, hash, auth); err != nil {
			return nil, err
		}
	}

	return &FetchResponse{ImageID: lastImg.ID}, nil
}

func (fetcher *RemoteV2Fetcher) fetchLayer(request *FetchRequest, img *image.Image, hash digest.Digest, auth *registry.RequestAuthorization) error {
	fetcher.GraphLock.Acquire(img.ID)
	defer fetcher.GraphLock.Release(img.ID)

	if !fetcher.Graph.Exists(img.ID) {
		reader, _, err := request.Session.GetV2ImageBlobReader(request.Endpoint, request.RemotePath, hash, auth)
		if err != nil {
			return FetchError("GetV2ImageBlobReader", request.Endpoint.URL.Host, request.Path, err)
		}
		defer reader.Close()

		err = fetcher.Graph.Register(img, reader)
		if err != nil {
			return FetchError("GraphRegister", request.Endpoint.URL.Host, request.Path, err)
		}
	}

	return nil
}
