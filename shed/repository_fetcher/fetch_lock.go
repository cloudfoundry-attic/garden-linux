package repository_fetcher

import (
	"fmt"
	"sync"
)

type FetchLock struct {
	locks map[string]*sync.Mutex
	mutex sync.Mutex
}

func NewFetchLock() *FetchLock {
	return &FetchLock{
		locks: make(map[string]*sync.Mutex),
	}
}

func (l *FetchLock) Acquire(key string) {
	var lock *sync.Mutex

	l.mutex.Lock()
	if _, ok := l.locks[key]; !ok {
		l.locks[key] = new(sync.Mutex)
	}

	lock = l.locks[key]
	l.mutex.Unlock()

	lock.Lock()
}

func (l *FetchLock) Release(key string) error {
	l.mutex.Lock()
	lock, ok := l.locks[key]
	l.mutex.Unlock()

	if !ok {
		return fmt.Errorf("repository_fetcher: releasing lock: no lock for key: %s", key)
	}

	lock.Unlock()

	return nil
}
