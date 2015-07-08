package repository_fetcher

import (
	"encoding/json"

	"github.com/docker/distribution/digest"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
)

type RemoteV2Fetcher struct {
	Graph     Graph
	GraphLock Lock
}

func (fetcher *RemoteV2Fetcher) Fetch(request *FetchRequest) (*FetchResponse, error) {
	auth := registry.NewRequestAuthorization(&cliconfig.AuthConfig{}, request.Endpoint, "", "", []string{})
	_, manifestBytes, err := request.Session.GetV2ImageManifest(request.Endpoint, request.Path, request.Tag, auth)
	if err != nil {
		return nil, FetchError("GetV2ImageManifest", request.Hostname, request.Path, err)
	}

	var manifest registry.ManifestData
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, FetchError("UnmarshalManifest", request.Hostname, request.Path, err)
	}

	var lastImg *image.Image

	for i := len(manifest.FSLayers) - 1; i >= 0; i-- {
		hash, err := digest.ParseDigest(manifest.FSLayers[i].BlobSum)
		if err != nil {
			return nil, FetchError("ParseDigest", request.Hostname, request.Path, err)
		}

		img, err := image.NewImgJSON([]byte(manifest.History[i].V1Compatibility))
		if err != nil {
			return nil, FetchError("NewImgJSON", request.Hostname, request.Path, err)
		}
		if i == 0 {
			lastImg = img
		}

		// TODO: Concurrent pulling?
		if !fetcher.Graph.Exists(img.ID) {
			reader, _, err := request.Session.GetV2ImageBlobReader(request.Endpoint, request.Path, hash, auth)
			if err != nil {
				return nil, FetchError("GetV2ImageBlobReader", request.Hostname, request.Path, err)
			}
			defer reader.Close()

			err = fetcher.Graph.Register(img, reader)
			if err != nil {
				return nil, FetchError("GraphRegister", request.Hostname, request.Path, err)
			}
		}
	}

	return &FetchResponse{ImageID: lastImg.ID}, nil
}
