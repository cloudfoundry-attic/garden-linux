package repository_fetcher

import (
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
)

//go:generate counterfeiter . RemoteImageIDFetcher
type RemoteImageIDFetcher interface {
	FetchID(u *url.URL) (layercake.ID, error)
}

type ImageRetainer struct {
	DirectoryRootfsIDProvider ContainerIDProvider
	DockerImageIDFetcher      RemoteImageIDFetcher
	GraphRetainer             layercake.Retainer
}

func (i *ImageRetainer) Retain(imageList []string) {
	rootfsURL, err := url.Parse(imageList[0])
	if err != nil {
		return
	}

	switch rootfsURL.Scheme {
	case "docker":
		id, err := i.DockerImageIDFetcher.FetchID(rootfsURL)
		if err != nil {
			return
		}

		i.GraphRetainer.Retain(id)
	default:
		i.GraphRetainer.Retain(i.DirectoryRootfsIDProvider.ProvideID(imageList[0]))
	}
}
