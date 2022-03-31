package syncromap

import (
	"fmt"
	"sync"
)

// Map is the basic interface for map-like objects
type Map[K comparable, V any] interface {
	Load(K) (V, bool)
	Store(K, V)
	Delete(K)
}

// Transaction is the map interface used within a transaction
type Transaction[K comparable, V any] interface {
	Map[K, V]
	Len() int
	Range(RangeFunc[K, V])
	EndTransaction()
}

// SyncroMap is a thread-safe generic map type
type SyncroMap[K comparable, V any] interface {
	Map[K, V]
	BeginTransaction() Transaction[K, V]
}

// RangeFunc is the type of the callback used in Range()
type RangeFunc[K comparable, V any] func(key K, value V) (abort bool)

// =================================================================================================

// dmap actually holds the data
type dmap[K comparable, V any] struct {
	data map[K]V
}

// Load a value from the map
func (m *dmap[K, V]) Load(key K) (value V, ok bool) {
	v, okv := m.data[key]
	return v, okv
}

// Store a value into the map
func (m *dmap[K, V]) Store(key K, value V) {
	m.data[key] = value
}

// Delete a value from the map
func (m *dmap[K, V]) Delete(key K) {
	delete(m.data, key)
}

// Len returns the length of the map
func (m *dmap[K, V]) Len() int {
	return len(m.data)
}

// Range calls a callback for each value in the map
func (m *dmap[K, V]) Range(r RangeFunc[K, V]) {
	for k, v := range m.data {
		abort := r(k, v)
		if abort {
			break
		}
	}
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

// Load loads a single value from the map
func (m *lmap[K, V]) Load(key K) (value V, ok bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.d.Load(key)
}

// Store stores a single value to the map
func (m *lmap[K, V]) Store(key K, value V) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.d.Store(key, value)
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

// Load loads a value from the map
func (m *tmap[K, V]) Load(key K) (value V, ok bool) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	return m.l.d.Load(key)
}

// Store stores a value to the map
func (m *tmap[K, V]) Store(key K, value V) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.l.d.Store(key, value)
}

// Delete deletes a value from the map
func (m *tmap[K, V]) Delete(key K) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.l.d.Delete(key)
}

// Len returns the length of the map
func (m *tmap[K, V]) Len() int {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	return m.l.d.Len()
}

// Range calls a callback for each value in the map
func (m *tmap[K, V]) Range(r RangeFunc[K, V]) {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.l.d.Range(r)
}

// EndTransaction ends the transaction
func (m *tmap[K, V]) EndTransaction() {
	if !m.transactionActive {
		panic(ErrTransAfterEndTrans)
	}
	m.transactionActive = false
	m.l.lock.Unlock()
}
