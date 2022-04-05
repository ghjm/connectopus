package syncromap

import (
	"fmt"
	"os"
	"sync"
)

// Map is the basic interface for map-like objects.
type Map[K comparable, V any] interface {
	// Get gets a value from the map
	Get(K) (V, bool)
	// Set sets a value in the map
	Set(K, V)
	// Create sets a value in the map, which must not already exist
	Create(K, V) error
	// Delete removes a value from the map, and is a no-op if the value doesn't exist
	Delete(K)
}

// SyncroMap is a thread-safe generic map type
type SyncroMap[K comparable, V any] interface {
	Map[K, V]
	BeginTransaction() Transaction[K, V]
}

// Transaction is the map interface used within a transaction
type Transaction[K comparable, V any] interface {
	Map[K, V]
	RawMap() *map[K]V
	EndTransaction()
}

// =================================================================================================

// dmap actually holds the data
type dmap[K comparable, V any] struct {
	data map[K]V
}

// Get a value from the map
func (m *dmap[K, V]) Get(key K) (value V, ok bool) {
	v, okv := m.data[key]
	return v, okv
}

// Set stores a value into the map
func (m *dmap[K, V]) Set(key K, value V) {
	m.data[key] = value
}

// Create stores a value into the map, which must not already exist
func (m *dmap[K, V]) Create(key K, value V) error {
	_, ok := m.data[key]
	if ok {
		return os.ErrExist
	}
	m.data[key] = value
	return nil
}

// Delete a value from the map
func (m *dmap[K, V]) Delete(key K) {
	delete(m.data, key)
}

// =================================================================================================

// lmap holds the lock that controls access to the underlying dmap
type lmap[K comparable, V any] struct {
	d    *dmap[K, V]
	lock *sync.RWMutex
}

// NewMap constructs a SyncroMap of the given type
func NewMap[K comparable, V any]() SyncroMap[K, V] {
	m := &lmap[K, V]{
		d: &dmap[K, V]{
			data: make(map[K]V),
		},
		lock: &sync.RWMutex{},
	}
	return m
}

// Get loads a single value from the map
func (m *lmap[K, V]) Get(key K) (value V, ok bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.d.Get(key)
}

// Set stores a single value to the map
func (m *lmap[K, V]) Set(key K, value V) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.d.Set(key, value)
}

// Create stores a single value to the map, which must not already exist
func (m *lmap[K, V]) Create(key K, value V) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.d.Create(key, value)
}

// Delete deletes a value from the map
func (m *lmap[K, V]) Delete(key K) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.d.Delete(key)
}

// BeginTransaction returns an object that allows multiple operations while blocking all other users of the map
func (m *lmap[K, V]) BeginTransaction() Transaction[K, V] {
	m.lock.Lock()
	return &tmap[K, V]{
		l:                 m,
		transactionActive: true,
	}
}

// =================================================================================================

// tmap manages a single transaction started by an lmap
type tmap[K comparable, V any] struct {
	l                 *lmap[K, V]
	transactionActive bool
}

// ErrTransAfterEndTrans is used when a transaction function panics due to the transaction having already been ended
var ErrTransAfterEndTrans = fmt.Errorf("transaction accessed after EndTransaction")

// Get loads a value from the map
func (m *tmap[K, V]) Get(key K) (value V, ok bool) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	return m.l.d.Get(key)
}

// Set stores a value to the map
func (m *tmap[K, V]) Set(key K, value V) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.l.d.Set(key, value)
}

// Create stores a value to the map, which must not already exist
func (m *tmap[K, V]) Create(key K, value V) error {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	return m.l.d.Create(key, value)
}

// Delete deletes a value from the map
func (m *tmap[K, V]) Delete(key K) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.l.d.Delete(key)
}

// RawMap returns a pointer to the raw map.  Must not be retained outside the transaction.
func (m *tmap[K, V]) RawMap() *map[K]V {
	return &m.l.d.data
}

// EndTransaction ends the transaction
func (m *tmap[K, V]) EndTransaction() {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.transactionActive = false
	m.l.lock.Unlock()
}
