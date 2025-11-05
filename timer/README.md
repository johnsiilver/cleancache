# Clean Cache (Timer)

[![GoDoc](https://godoc.org/github.com/johnsiilver/cleancache/timer?status.svg)](https://pkg.go.dev/github.com/johnsiilver/cleancache/timer)
[![Go Report Card](https://goreportcard.com/badge/github.com/johnsiilver/cleancache/timer)](https://goreportcard.com/report/github.com/johnsiilver/cleancache/timer)

**NOTE** While you can use the code here, the code is simply for a medium article.

# Introduction

This package is similar to the root version, except it cleans up values vs a loop.

* Uses weak pointers to automatically collect values no longer in use
* Automatically delete keys for values no longer in use at intervals
* Shrinks the underlying map when values are deleted
* Uses sharded maps for better concurrency
* Thread-safe

# Use

It is important to remember this values stay in the cache until a value is no longer in use. Once that occurs, it will live in the cache until a garbage collection occurs

This means that the cache is optimized for storage size and constant use.

There is no maximum size, either the values are being used and remain in the cache during collection or they aren't, where caches without weak pointers can't determine if something is in use and rely on TTL and LRU semantics generally.

This cache relies on pointer values. Heavy parallel set and get times will be more expensive than using maps with locks, sync.Map or something like bigcache, but less than the version at the repo root.

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
