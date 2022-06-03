package syncro

import (
	"fmt"
	"sync"
)

// Map is a thread-safe generic map type
type Map[K comparable, V any] struct {
	value map[K]V
	lock  sync.RWMutex
}

func (m *Map[K, V]) createIfNil() {
	if m.value == nil {
		m.value = make(map[K]V)
	}
}

// NewMap constructs a new syncro.Map, initialized from a Go map.  It is not necessary to use NewMap if you
// do not need to set an initial value.
func NewMap[K comparable, V any](initMap map[K]V) Map[K, V] {
	return Map[K, V]{
		value: initMap,
	}
}

// Get a value from the map.  If it exists, returns the value and true; otherwise, returns the zero value and false.
func (m *Map[K, V]) Get(key K) (V, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	v, ok := m.value[key]
	return v, ok
}

// Set stores a value into the map
func (m *Map[K, V]) Set(key K, value V) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.createIfNil()
	m.value[key] = value
}

var ErrAlreadyExists = fmt.Errorf("map key already exists")

// Create stores a value into the map, which must not already exist.  Returns ErrAlreadyExists if the key exists.
func (m *Map[K, V]) Create(key K, value V) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.createIfNil()
	_, ok := m.value[key]
	if ok {
		return ErrAlreadyExists
	}
	m.value[key] = value
	return nil
}

// Delete a value from the map
func (m *Map[K, V]) Delete(key K) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.createIfNil()
	delete(m.value, key)
}

// WorkWith calls a function to work with the map under lock
func (m *Map[K, V]) WorkWith(f func(*map[K]V)) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.createIfNil()
	f(&m.value)
}

// WorkWithReadOnly calls a function to work with the map under lock.  You are on the honor system not to change it.
func (m *Map[K, V]) WorkWithReadOnly(f func(map[K]V)) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	m.createIfNil()
	f(m.value)
}
