// Package cleancache provides a thread-safe weak pointer cache that automatically cleans up entries
// when the weakly referenced objects are garbage collected. It supports basic operations like Get, Set,
// CompareAndSwap, and CompareAndDelete, with customizable equality checks. It uses a shared map for concurrency
// that also shrinks with deleted keys. The sharded map is based on Tidwall's shardedmap implementation, but updated for
// generics and uses the maphash package. The cache has no size limit and relies on Go's runtime to manage memory
// once objects are no longer referenced. This uses a custom version of github.com/gostdlib/concurrency/sync's shardedmap
// for slightly better performance.
package cleancache

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/gostdlib/base/context"
	"github.com/johnsiilver/cleancache/internal/shardmap"
	"github.com/johnsiilver/cleancache/internal/shardmap/hashmap"
)

type ttlEntry[V any] struct {
	hold  time.Time
	value *V
}

// Cache is a weak pointer cache.
type Cache[K comparable, V any] struct {
	m        shardmap.Map[K, V]
	ttl      time.Duration
	interval time.Duration

	ttlLock sync.Mutex
	ttlMap  hashmap.Map[K, ttlEntry[V]]
}

type opts struct {
	ttl      time.Duration
	interval time.Duration
}

// Option is an option for New().
type Option func(o opts) (opts, error)

// WithTTL sets the time-to-live for entries in the cache and the cleanup interval.
// Entries older than ttl will be removed during cleanup.
// The cleanup interval must be at least 1 second.
// If ttl is 0, an error is returned.
func WithTTL(ttl, interval time.Duration) Option {
	return func(o opts) (opts, error) {
		if interval < 1*time.Second {
			return o, fmt.Errorf("cleanup interval must be at least 1 second")
		}
		if ttl == 0 {
			return o, fmt.Errorf("ttl must be greater than 0")
		}
		o.ttl = ttl
		o.interval = interval
		return o, nil
	}
}

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
	c := &Cache[K, V]{}
	if o.ttl > 0 {
		c.ttl = o.ttl
		c.interval = o.interval
		_ = context.Pool(ctx).Submit(
			ctx,
			func() {
				c.ttlExpire(ctx)
			},
		)
	}

	return c, nil
}

// ttlExpire runs in a background goroutine to clean up expired entries in the ttlMap.
// This map is holding values with a regular pointer to prevent the weak reference from
// being collected before the ttl expires. Once the TTL expires, the entry is deleted from the ttlMap,
// which allows the weak reference in the main map to be collected by the GC if not used.
func (m *Cache[K, V]) ttlExpire(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	deletions := []K{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			m.ttlLock.Lock()
			for k, v := range m.ttlMap.All() {
				if v.hold.Before(now) {
					deletions = append(deletions, k)
				}
			}
			for _, k := range deletions {
				m.ttlMap.Delete(k)
			}
			m.ttlLock.Unlock()
			deletions = deletions[:0]
		}
	}
}

// Set assigns a value to a key.
// Returns the previous value, or false when no value was assigned. If you
// Set a nil value, it is equivalent to Delete.
func (m *Cache[K, V]) Set(k K, v *V) (prev *V, replaced bool) {
	if v == nil {
		prev, deleted := m.Del(k)
		return prev, deleted
	}
	if m.ttl > 0 {
		m.ttlLock.Lock()
		m.ttlMap.Set(k, ttlEntry[V]{hold: time.Now().Add(m.ttl), value: v})
		m.ttlLock.Unlock()
	}

	prev, replaced = m.m.Set(k, v)
	runtime.AddCleanup[V, K](
		v,
		func(k K) {
			m.m.DeleteIfNil(k)
		},
		k,
	)

	if !replaced {
		return nil, false
	}
	if prev == nil {
		return nil, false
	}
	return prev, replaced
}

// Get returns a value for a key.
// Returns false when no value has been assign for key.
func (m *Cache[K, V]) Get(k K) (value *V, ok bool) {
	return m.m.Get(k)
}

// Del deletes a value for a key.
// Returns the deleted value, or false when no value was assigned.
func (m *Cache[K, V]) Del(k K) (prev *V, deleted bool) {
	if m.ttl > 0 {
		m.ttlLock.Lock()
		m.ttlMap.Delete(k)
		m.ttlLock.Unlock()
	}
	return m.m.Delete(k)
}

// Len returns the number of values in map. This is an approximation since keys may hold nil values that
// have not yet been cleaned up.
func (m *Cache[K, V]) Len() int {
	return m.m.Len()
}
