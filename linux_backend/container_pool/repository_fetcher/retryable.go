package repository_fetcher

import "log"

type Retryable struct {
	RepositoryFetcher
}

func (retryable Retryable) Fetch(repoName string, tag string) (string, error) {
	var res string
	var err error

	for attempt := 1; attempt <= 3; attempt++ {
		res, err = retryable.RepositoryFetcher.Fetch(repoName, tag)
		if err == nil {
			break
		}

		log.Println("attempt", attempt, "of 3 failed:", err)
	}

	return res, err
}
