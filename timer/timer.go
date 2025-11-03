// Package timer offers a weak pointer cache that automatically cleans up entries based on a timer.
// It uses a sharded map for concurrency and periodically removes entries whose weak pointers have been cleared.
// The sharded map allows us to delete expired entries without locking the entire map, improving performance in concurrent scenarios.
package timer

import (
	"runtime"
	"time"

	"github.com/gostdlib/base/context"
	"github.com/johnsiilver/cleancache/internal/shardmap"
)

// Cache is a weakPointer cache. It provides a cache of objects that have weak pointers to them.
// When the weak pointer's value becomes nil, we delete the key.
// This cache is thread-safe.
type Cache[K comparable, V any] struct {
	shardedMap *shardmap.Map[K, V]
	lastClean  time.Time
}

type opts struct {
	cleanupInterval time.Duration
}

// Option is an option for New().
type Option func(opts) (opts, error)

// WithCleanupInterval sets the interval at which the cache will clean up expired entries.
// The default is 30 seconds.
func WithCleanupInterval(d time.Duration) Option {
	return func(o opts) (opts, error) {
		o.cleanupInterval = d
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

	c := &Cache[K, V]{
		shardedMap: &shardmap.Map[K, V]{},
		lastClean:  time.Now(),
	}

	t := time.NewTicker(30 * time.Second)
	ctx, cancel := context.WithCancel(ctx)
	_ = context.Pool(ctx).Submit(
		ctx,
		func() {
			for {
				select {
				case <-ctx.Done():
					t.Stop()
					return
				case <-t.C:
					c.shardedMap.CleanShards()
				}
			}
		},
	)

	runtime.AddCleanup(
		c,
		func(struct{}) {
			cancel()
		},
		struct{}{},
	)

	return c, nil
}

// Len returns the number of entries in the cache. This is an approximate value, as entries may be
// removed asynchronously when their weak pointers are cleared, but the keys have not yet been deleted.
func (c *Cache[K, V]) Len() int {
	return c.shardedMap.Len()
}

// Get retrieves a value at Key.
func (c *Cache[K, V]) Get(k K) (value *V, ok bool) {
	if c == nil || c.shardedMap == nil {
		return nil, false
	}

	return c.shardedMap.Get(k)
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

	return c.shardedMap.Set(k, v)
}

// delete removes a planID from the cache.
func (c *Cache[K, V]) Del(k K) (prev *V, ok bool) {
	if c == nil || c.shardedMap == nil {
		return
	}

	return c.shardedMap.Delete(k)
}
