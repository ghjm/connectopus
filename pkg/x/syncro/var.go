package syncro

import (
	"sync"
)

// Var stores a single variable and allows synchronized access
type Var[T any] struct {
	value T
	lock  sync.RWMutex
}

// NewVar creates a new variable with a given initial value.  It is not required to use NewVar if you don't
// need to set an initial value.
func NewVar[T any](value T) Var[T] {
	return Var[T]{value: value}
}

// Set sets the value
func (sv *Var[T]) Set(value T) {
	sv.lock.Lock()
	defer sv.lock.Unlock()
	sv.value = value
}

// Get retrieves the value
func (sv *Var[T]) Get() T {
	sv.lock.RLock()
	defer sv.lock.RUnlock()
	return sv.value
}

// WorkWith calls a function to work with the data under lock
func (sv *Var[T]) WorkWith(f func(*T)) {
	sv.lock.Lock()
	defer sv.lock.Unlock()
	f(&sv.value)
}

// WorkWithReadOnly calls a function to work with the data under lock.  You are on the honor system not to change it.
func (sv *Var[T]) WorkWithReadOnly(f func(T)) {
	sv.lock.RLock()
	defer sv.lock.RUnlock()
	f(sv.value)
}
