// Package cleancache provides a thread-safe weak pointer cache that automatically cleans up entries
// when the weakly referenced objects are garbage collected. It supports basic operations like Get, Set,
// CompareAndSwap, and CompareAndDelete, with customizable equality checks. It uses a shared map for concurrency
// that also shrinks with deleted keys. The sharded map is based on Tidwall's shardedmap implementation, but updated for
// generics and uses the maphash package. The cache has no size limit and relies on Go's runtime to manage memory
// once objects are no longer referenced. This uses a custom version of github.com/gostdlib/concurrency/sync's shardedmap
// for slightly better performance.
package cleancache

import (
	"context"
	"hash/maphash"
	"iter"
	"runtime"
	"sync/atomic"
	"weak"

	"github.com/gostdlib/base/concurrency/sync"
	rhh "github.com/johnsiilver/cleancache/internal/shardmap/hashmap"
)

// Map is a hashmap. Like map[string]interface{}, but sharded and thread-safe.
type Cache[K comparable, V any] struct {
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

type opts struct{}

// Option is an option for New().
type Option func(opts) (opts, error)

// New creates a new Cache with the given options.
func New[K comparable, V any](ctx context.Context, options ...Option) (*Cache[K, V], error) {
	o := opts{}
	for _, option := range options {
		var err error
		o, err = option(o)
		if err != nil {
			return nil, err
		}
	}

	c := &Cache[K, V]{cap: 1024}

	c.shards = runtime.NumCPU() * 16
	if c.shards%2 == 1 {
		c.shards++
	}

	scap := c.cap / c.shards
	c.mus = make([]sync.RWMutex, c.shards)
	c.maps = make([]*rhh.Map[K, weak.Pointer[V]], c.shards)
	for i := 0; i < len(c.maps); i++ {
		c.maps[i] = rhh.New[K, weak.Pointer[V]](scap)
	}
	c.seed = maphash.MakeSeed()

	return c, nil
}

// Clear out all values from map
func (m *Cache[K, V]) Clear() {
	for i := 0; i < m.shards; i++ {
		m.mus[i].Lock()
		m.maps[i] = rhh.New[K, weak.Pointer[V]](m.cap / m.shards)
		m.mus[i].Unlock()
	}
}

// Set assigns a value to a key.
// Returns the previous value, or false when no value was assigned. If you
// Set a nil value, it is equivalent to Delete.
func (m *Cache[K, V]) Set(k K, v *V) (prev *V, replaced bool) {
	if v == nil {
		return m.Del(k)
	}
	shard := m.choose(k)

	n := weak.Make(v)
	runtime.AddCleanup[V, K](
		v,
		func(k K) {
			m.Del(k)
		},
		k,
	)
	m.mus[shard].Lock()
	wp, replaced := m.maps[shard].Set(k, n)
	m.mus[shard].Unlock()
	prev = wp.Value()
	if replaced && prev != nil {
		return prev, replaced
	}
	return prev, false
}

// Get returns a value for a key.
// Returns false when no value has been assign for key.
func (m *Cache[K, V]) Get(k K) (value *V, ok bool) {
	shard := m.choose(k)
	m.mus[shard].RLock()
	wp, ok := m.maps[shard].Get(k)
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
func (m *Cache[K, V]) Del(k K) (prev *V, deleted bool) {
	shard := m.choose(k)
	m.mus[shard].Lock()
	wp, deleted := m.maps[shard].Delete(k)
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

// Len returns the number of values in map. This is an approximation since keys may hold nil values that
// have not yet been cleaned up.
func (m *Cache[K, V]) Len() int {
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
func (m *Cache[K, V]) all() iter.Seq2[K, *V] {
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

func (m *Cache[K, V]) choose(key K) int {
	return int(maphash.Comparable(m.seed, key) & uint64(m.shards-1))
}
