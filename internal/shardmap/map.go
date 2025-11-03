// Package shardmap is a re-done version of Josh Baker's shardmap package. It switches out the hash
// from xxhash to maphash, uses generics and has a few other minor changes. It is a thread-safe.
// Based on Josh Baker's shardmap.
package shardmap

import (
	"hash/maphash"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

	rhh "github.com/johnsiilver/cleancache/internal/shardmap/hashmap"
)

// Map is a hashmap. Like map[string]interface{}, but sharded and thread-safe.
type Map[K comparable, V any] struct {
	// IsEqual is a function that determines if two values are equal. This is not required unless using
	// CompareAndSwap or CompareAndDelete.
	IsEqual func(old, new V) bool
	init    sync.Once
	cap     int
	shards  int
	mus     []sync.RWMutex
	maps    []*rhh.Map[K, weak.Pointer[V]]

	count atomic.Uint64

	seed maphash.Seed
}

// New returns a new hashmap with the specified capacity. This function is only
// needed when you must define a minimum capacity, otherwise just use:
//
//	var m shardmap.Map
func New[K comparable, V any](cap int) *Map[K, V] {
	return &Map[K, V]{cap: cap}
}

// Clear out all values from map
func (m *Map[K, V]) Clear() {
	m.initDo()
	for i := 0; i < m.shards; i++ {
		m.mus[i].Lock()
		m.maps[i] = rhh.New[K, weak.Pointer[V]](m.cap / m.shards)
		m.mus[i].Unlock()
	}
}

// Set assigns a value to a key.
// Returns the previous value, or false when no value was assigned.
func (m *Map[K, V]) Set(key K, value *V) (prev *V, replaced bool) {
	m.initDo()
	shard := m.choose(key)
	m.mus[shard].Lock()
	wp, replaced := m.maps[shard].Set(key, weak.Make(value))
	m.mus[shard].Unlock()
	prev = wp.Value()
	if replaced && prev != nil {
		return prev, replaced
	}
	return prev, false
}

// Get returns a value for a key.
// Returns false when no value has been assign for key.
func (m *Map[K, V]) Get(key K) (value *V, ok bool) {
	m.initDo()
	shard := m.choose(key)
	m.mus[shard].RLock()
	wp, ok := m.maps[shard].Get(key)
	m.mus[shard].RUnlock()
	if !ok {
		return nil, false
	}
	value = wp.Value()
	if value == nil {
		return nil, false
	}
	return value, ok
}

// Delete deletes a value for a key.
// Returns the deleted value, or false when no value was assigned.
func (m *Map[K, V]) Delete(key K) (prev *V, deleted bool) {
	m.initDo()
	shard := m.choose(key)
	m.mus[shard].Lock()
	wp, deleted := m.maps[shard].Delete(key)
	if !deleted {
		m.mus[shard].Unlock()
		return nil, deleted
	}
	prev = wp.Value()
	if prev == nil {
		m.mus[shard].Unlock()
		return nil, false
	}
	m.mus[shard].Unlock()
	return prev, deleted
}

// CleanShards removes all entries with nil values from the map.
func (m *Map[K, V]) CleanShards() {
	m.initDo()
	for shard := range m.maps {
		m.mus[shard].Lock()
		for k, v := range m.maps[shard].All() {
			if v.Value() == nil {
				m.maps[shard].Delete(k)
			}
		}
		m.mus[shard].Unlock()
	}
}

// Len returns the number of values in map. This is an approximation since keys may hold nil values that
// have not yet been cleaned up.
func (m *Map[K, V]) Len() int {
	m.initDo()
	var len int
	for i := 0; i < m.shards; i++ {
		m.mus[i].Lock()
		len += m.maps[i].Len()
		m.mus[i].Unlock()
	}
	return len
}

// All returns a sequence of all key/values. It is not safe to call
// Set or Delete while iterating.
func (m *Map[K, V]) all() iter.Seq2[K, *V] {
	m.initDo()
	return func(yield func(K, *V) bool) {
		for i := 0; i < m.shards; i++ {
			for k, wp := range m.maps[i].All() {
				v := wp.Value()
				if v == nil {
					continue
				}
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

func (m *Map[K, V]) choose(key K) int {
	return int(maphash.Comparable(m.seed, key) & uint64(m.shards-1))
}

func (m *Map[K, V]) initDo() {
	m.init.Do(func() {
		m.shards = 1
		for m.shards < runtime.NumCPU()*16 {
			m.shards *= 2
		}
		scap := m.cap / m.shards
		m.mus = make([]sync.RWMutex, m.shards)
		m.maps = make([]*rhh.Map[K, weak.Pointer[V]], m.shards)
		for i := 0; i < len(m.maps); i++ {
			m.maps[i] = rhh.New[K, weak.Pointer[V]](scap)
		}
		m.seed = maphash.MakeSeed()
	})
}
