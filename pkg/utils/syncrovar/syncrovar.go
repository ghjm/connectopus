package syncrovar

import (
	"sync"
)

// SyncroVar stores a single variable and allows synchronized access
type SyncroVar[T any] struct {
	value T
	lock  sync.RWMutex
}

// Set sets the value
func (sv *SyncroVar[T]) Set(value T) {
	sv.lock.Lock()
	defer sv.lock.Unlock()
	sv.value = value
}

// Get retrieves the value
func (sv *SyncroVar[T]) Get() T {
	sv.lock.RLock()
	defer sv.lock.RUnlock()
	return sv.value
}

// WorkWith calls a function to work with the data under lock
func (sv *SyncroVar[T]) WorkWith(f func(*T)) {
	sv.lock.Lock()
	defer sv.lock.Unlock()
	f(&sv.value)
}

// WorkWithReadOnly calls a function to work with the data under lock.  You are on the honor system not to change it.
func (sv *SyncroVar[T]) WorkWithReadOnly(f func(*T)) {
	sv.lock.RLock()
	defer sv.lock.RUnlock()
	f(&sv.value)
}
