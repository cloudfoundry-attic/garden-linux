package repository_fetcher

import (
	"errors"
	"io"
)

type QuotaedReader struct {
	R io.Reader
	N int64
}

var ErrQuotaExceeded = errors.New("quota exceeded")

func (q *QuotaedReader) Read(p []byte) (int, error) {
	if int64(len(p)) > q.N {
		p = p[0:q.N]
	}

	n, err := q.R.Read(p)
	q.N = q.N - int64(n)

	if q.N <= 0 {
		return n, ErrQuotaExceeded
	}

	return n, err
}
