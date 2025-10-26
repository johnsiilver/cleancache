# Clean Cache

[![GoDoc](https://godoc.org/github.com/johnsiilver/cleancache?status.svg)](https://pkg.go.dev/github.com/johnsiilver/cleancache)
[![Go Report Card](https://goreportcard.com/badge/github.com/johnsiilver/cleancache)](https://goreportcard.com/report/github.com/johnsiilver/cleancache)

A thread-safe weakpointer cache that automatically cleans keys deleted weakpointer values without fancy background mechanics. It
simplifies weakpointer map cleanup and provides shrinking maps vs. non-shrinking.

# Introduction

This package introduces a cache that:

* Uses weak pointers to automatically collect values no longer in use
* Automatically delete keys for values no longer in use, removing the need to lock and loop the cache at various times
* Shrinks the underlying map when values are deleted
* Thread-safe

This uses a sharded map implementation based Josh Baker's excellent sharded map: https://github.com/tidwall/shardmap

This sharded map is updated to utilize generics and Go's maphash, which didn't exist when Josh made his.

# Use

It is important to remember this isn't a cache based on TTLs or least-recently used semantics. Values stay in the cache until a value is no longer in use. Once that occurs, it will live in the cache until a garbage collection occurs.

This means that the cache is optimized for storage size and constant use. If your cache needs a value to live for some lifetime this might is not your cache. This will have some LRU like semantics in that if it is in use at a GC cycle, it will stay around. But if the value isn't help by anyone, even if it has been used recently, it will be collectoed.

There is no maximum size, either the values are being used and remain in the cache during collection or they aren't, where caches without weak pointers can't determine if something is in use, only the last time the value was fetched from the cache.

This cache relies on pointer values and uses runtime calls. Heavy parallel set and get times will be more expensive than using maps with locks, sync.Map or a something like bigcache due to runtime call overhead. Memory use should be less due to a shrinking map implementation and GC evictions.

The following show the semantics for using the cache:

## Create the cache
```go
cache, err := New[int, Data](ctx)
if err != nil {
	panic(err)
}
```

## Set a value in the cache
```go
id := 0
d := &Data{Name: "John"}
oldValue, oldValueExisted := cache.Set(id, d)
```

## Get a value from the cache
```go
value, ok := cache.Get(id)
if !ok {
	return fmt.Errorf("value not in cache")
}
```

## Delete a value form the cache
```go
// Old is the value you deleted, ok is true if the value was deleted vs just not found.
old, ok := cache.Del(id)
```

# Usage note

The code here is simplistic, relying on recent advancements in the Go standard library. There is < 200 lines of code. I have not
spent time trying to optimize benchmarks. I'm sure there are numerous enhancements that could be done. I have a few cases where this will be useful as it is.

If your looking for an LRU, I like this one: https://github.com/tidwall/tinylru .
