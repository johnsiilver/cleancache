# Clean Cache

[![GoDoc](https://godoc.org/github.com/johnsiilver/cleancache?status.svg)](https://pkg.go.dev/github.com/johnsiilver/cleancache)
[![Go Report Card](https://goreportcard.com/badge/github.com/johnsiilver/cleancache)](https://goreportcard.com/report/github.com/johnsiilver/cleancache)

This repo provides various code examples of weak pointer caches that have different methods for cleanup and features.

This document refers to the root cache implementation and not the ones in the sub-directories.



The root package simplifies weak pointer map cleanup using runtime.AddCleanup() and all provide shrinking map implementations vs. standard maps that cannot shrink in size.

**NOTE** While you can use the code here, the code is simply for a medium article.

# Introduction

This package introduces several different caches that:

* Uses weak pointers to automatically collect values no longer in use
* Automatically delete keys for values no longer in use via runtime.AddCleanup()`
* Shrinks the underlying map when values are deleted
* Uses sharded maps for better concurrency
* Can provide a minimum TTL via an option
* Can utilize a singleflight package to do a single fetch if multiple get requests for the same key occur
* Thread-safe

This uses a sharded map implementations based Josh Baker's excellent sharded map: https://github.com/tidwall/shardmap

The sharded map is updated to utilize generics and Go's maphash, which didn't exist when Josh made his.

# Use

It is important to remember this values stay in the cache until a value is no longer in use. Once that occurs, it will live in the cache until a garbage collection occurs. If you use the TTL option, it will at least stay that long.

This means that the cache is optimized for storage size and constant use.

There is no maximum size, either the values are being used and remain in the cache during collection or they aren't, where caches without weak pointers can't determine if something is in use and rely on TTL and LRU semantics generally.

This cache relies on pointer values and uses runtime calls. Heavy parallel set and get times will be more expensive than using maps with locks, sync.Map or something like bigcache due to runtime call overhead. Memory use should be less due to a shrinking map implementation and automatic GC evictions.

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

The code here is simplistic, relying on recent advancements in the Go standard library. There is ~200 lines of code. I have not
spent time trying to optimize benchmarks. I'm sure there are numerous enhancements that could be done. I have a few cases where this will be useful as it is.

If your looking for an LRU, I like this one: https://github.com/tidwall/tinylru .

# Benchmark notes

I benchmarked these in a separate repo. Performance is less than things like bigcache in high concurrenct scenarios, sometimes multiples slower.  In single cache fetches its several times faster.

But overall, its hard to get a feel for how much overhead the runtime is going to add vs the collection times that are inherent in other caches.  A lot of this is also going to depend on how often the GC is running.  Weakpointers and cleanup calls are going to cost more in the runtime.

Micro benchmarks are going to be hard to tell what the overall effect on a system is going to be. Trying under realistic conditions for your service is going to give you the best watermarks on performance (cpu/memory/latency).

This package and all weak pointer caches are optimized for memory.
