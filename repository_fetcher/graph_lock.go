package repository_fetcher

import (
	"fmt"
	"sync"
)

type GraphLock struct {
	locks map[string]*Consumers
	mutex sync.Mutex
}

type Consumers struct {
	Cond  *sync.Cond
	Count int
}

func NewGraphLock() *GraphLock {
	return &GraphLock{
		locks: make(map[string]*Consumers),
	}
}

func (l *GraphLock) Acquire(key string) {
	l.mutex.Lock()
	if _, ok := l.locks[key]; !ok {
		l.locks[key] = new(Consumers)
		l.locks[key].Cond = sync.NewCond(new(sync.Mutex))
	}

	l.locks[key].Cond.L.Lock()
	defer l.locks[key].Cond.L.Unlock()

	var cond *sync.Cond
	l.locks[key].Count++
	if l.locks[key].Count > 1 {
		cond = l.locks[key].Cond
	}
	l.mutex.Unlock()

	if cond != nil {
		cond.Wait()
	}
}

func (l *GraphLock) Release(key string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	_, ok := l.locks[key]
	if !ok {
		return fmt.Errorf("repository_fetcher: releasing lock: no lock for key: %s", key)
	}

	l.locks[key].Cond.L.Lock()
	l.locks[key].Count--
	if l.locks[key].Count > 0 {
		l.locks[key].Cond.L.Unlock()
		l.locks[key].Cond.Signal()
	} else {
		delete(l.locks, key)
	}

	return nil
}
