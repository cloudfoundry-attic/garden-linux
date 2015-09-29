package repository_fetcher

import (
	"net/url"

	"github.com/pivotal-golang/lager"
)

type Retryable struct {
	RepositoryFetcher interface {
		Fetch(*url.URL, int64) (*Image, error)
	}

	Logger lager.Logger
}

func (retryable Retryable) Fetch(repoName *url.URL, diskQuota int64) (*Image, error) {
	var err error
	var response *Image
	for attempt := 1; attempt <= 3; attempt++ {
		response, err = retryable.RepositoryFetcher.Fetch(repoName, diskQuota)
		if err == nil {
			break
		}

		retryable.Logger.Error("failed-to-fetch", err, lager.Data{
			"attempt": attempt,
			"of":      3,
		})
	}

	return response, err
}
