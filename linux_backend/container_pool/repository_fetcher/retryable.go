package repository_fetcher

import (
	"github.com/pivotal-golang/lager"
)

type Retryable struct {
	RepositoryFetcher
}

func (retryable Retryable) Fetch(logger lager.Logger, repoName string, tag string) (string, error) {
	var res string
	var err error

	for attempt := 1; attempt <= 3; attempt++ {
		res, err = retryable.RepositoryFetcher.Fetch(logger, repoName, tag)
		if err == nil {
			break
		}

		logger.Error("failed-to-fetch", err, lager.Data{
			"attempt": attempt,
			"of":      3,
		})
	}

	return res, err
}
