package repository_fetcher

import (
	"net/url"

	"github.com/cloudfoundry-incubator/garden-linux/layercake"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . RemoteImageIDFetcher
type RemoteImageIDFetcher interface {
	FetchID(u *url.URL) (layercake.ID, error)
}

type ImageRetainer struct {
	DirectoryRootfsIDProvider ContainerIDProvider
	DockerImageIDFetcher      RemoteImageIDFetcher
	GraphRetainer             layercake.Retainer
	NamespaceCacheKey         string

	Logger lager.Logger
}

func (i *ImageRetainer) Retain(imageList []string) {
	log := i.Logger.Session("retain")
	log.Info("starting")
	defer log.Info("retained")

	for _, image := range imageList {
		log := log.WithData(lager.Data{"url": image})
		log.Info("retaining")

		rootfsURL, err := url.Parse(image)
		if err != nil {
			log.Error("parse", err)
			continue
		}

		var id layercake.ID
		if id, err = i.toID(rootfsURL); err != nil {
			log.Error("convert", err)
			continue
		}

		i.GraphRetainer.Retain(id)
		i.GraphRetainer.Retain(layercake.NamespacedID(id, i.NamespaceCacheKey))

		log.Info("retaining-complete")
	}
}

func (i *ImageRetainer) toID(u *url.URL) (id layercake.ID, err error) {
	switch u.Scheme {
	case "docker":
		return i.DockerImageIDFetcher.FetchID(u)
	default:
		return i.DirectoryRootfsIDProvider.ProvideID(u.Path), nil
	}
}
