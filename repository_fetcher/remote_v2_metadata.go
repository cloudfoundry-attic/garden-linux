package repository_fetcher

import (
	"encoding/json"

	"github.com/docker/distribution/digest"
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
)

type ImageV2MetadataProvider struct{}

func (provider *ImageV2MetadataProvider) ProvideImageID(request *FetchRequest) (string, error) {
	metadata, err := provider.ProvideMetadata(request)
	if err != nil {
		return "", err
	}
	return metadata.Images[0].ID, nil
}

func (provider *ImageV2MetadataProvider) ProvideMetadata(request *FetchRequest) (*ImageV2Metadata, error) {
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

	return &ImageV2Metadata{
		Images:        images,
		ImagesDigest:  hashes,
		Authorization: auth}, nil
}
