package repository_fetcher

import "fmt"

type ImageV1MetadataProvider struct{}

func (provider *ImageV1MetadataProvider) ProvideMetadata(request *FetchRequest) (*ImageV1Metadata, error) {
	request.Logger.Debug("docker-v1-fetch")

	repoData, err := request.Session.GetRepositoryData(request.Path)
	if err != nil {
		return nil, FetchError("GetRepositoryData", request.Endpoint.URL.Host, request.Path, err)
	}

	tagsList, err := request.Session.GetRemoteTags(repoData.Endpoints, request.Path)
	if err != nil {
		return nil, FetchError("GetRemoteTags", request.Endpoint.URL.Host, request.Path, err)
	}

	imgID, ok := tagsList[request.Tag]
	if !ok {
		return nil, FetchError("looking up tag", request.Endpoint.URL.Host, request.Path, fmt.Errorf("unknown tag: %v", request.Tag))
	}

	return &ImageV1Metadata{
		ImageID:   imgID,
		Endpoints: repoData.Endpoints}, nil
}
