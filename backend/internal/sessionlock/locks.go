package sessionlock

import "sync"

type Locks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func New() *Locks {
	return &Locks{locks: map[string]*sync.Mutex{}}
}

func (l *Locks) Lock(id string) func() {
	l.mu.Lock()
	lock := l.locks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		l.locks[id] = lock
	}
	l.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}
