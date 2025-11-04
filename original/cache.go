// Package cleancache provides a thread-safe weak pointer cache that automatically cleans up entries
// when the weakly referenced objects are garbage collected. It supports basic operations like Get, Set,
// CompareAndSwap, and CompareAndDelete, with customizable equality checks. It uses a shared map for concurrency
// that also shrinks with deleted keys. The sharded map is based on Tidwall's shardedmap implementation, but updated for
// generics and uses the maphash package. The cache has no size limit and relies on Go's runtime to manage memory
// once objects are no longer referenced. Unlike the base version at cleancache/cache.go, this version does not use a
// custom hashmap implementation, but instead uses a sharded map from the gostdlib library.  These are virtually identical
// except the other optimizes a few calls at a lower level. This version is easier to maintain.
package cleancache

import (
	"context"
	"runtime"
	"weak"

	"github.com/gostdlib/base/concurrency/sync"
)

// Cache is a weakPointer cache. It provides a cache of objects that have weak pointers to them.
// When the weak pointer's value becomes nil, we delete the key.
// This cache is thread-safe.
type Cache[K comparable, V any] struct {
	shardedMap *sync.ShardedMap[K, weak.Pointer[V]]
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

	c := &Cache[K, V]{shardedMap: &sync.ShardedMap[K, weak.Pointer[V]]{}}

	return c, nil
}

// Len returns the number of entries in the cache.
func (c *Cache[K, V]) Len() int {
	return c.shardedMap.Len()
}

// Get retrieves a value at Key.
func (c *Cache[K, V]) Get(k K) (value *V, ok bool) {
	if c == nil || c.shardedMap == nil {
		return nil, false
	}

	wp, ok := c.shardedMap.Get(k)
	if !ok {
		return nil, false
	}

	v := wp.Value()
	if v == nil {
		return nil, false
	}

	return v, true
}

// Set stores a key with value. If the key already exists, this will overwrite it. In that case
// we return the previous value and true. If v is nil, we delete the key and return the previous value.
func (c *Cache[K, V]) Set(k K, v *V) (Prev *V, ok bool) {
	if c == nil || c.shardedMap == nil {
		return
	}
	if v == nil {
		return c.Del(k)
	}

	wp := weak.Make(v)

	runtime.AddCleanup[V, K](
		v,
		func(k K) {
			c.Del(k)
		},
		k,
	)

	oldWP, ok := c.shardedMap.Set(k, wp)
	if !ok {
		return nil, false
	}
	oldV := oldWP.Value()
	if oldV == nil {
		return nil, false
	}

	return oldV, ok
}

// Del deletes a value for a key.
// Returns the deleted value, or false when no value was assigned.
func (c *Cache[K, V]) Del(k K) (prev *V, ok bool) {
	if c == nil || c.shardedMap == nil {
		return
	}

	wp, ok := c.shardedMap.Del(k)
	if !ok {
		return nil, false
	}
	v := wp.Value()
	if v == nil {
		return nil, false
	}
	return v, ok
}
