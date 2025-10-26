# Clean Cache

[![GoDoc](https://godoc.org/github.com/johnsiilver/cleancache?status.svg)](https://pkg.go.dev/github.com/johnsiilver/cleancache)
[![Go Report Card](https://goreportcard.com/badge/github.com/johnsiilver/cleancache)](https://goreportcard.com/report/github.com/johnsiilver/cleancache)

A thread-safe weakpointer cache that automatically cleans keys of deleted weakpointer values without looping through the cache.

# Introduction

This package introduces a cache that:

* Uses weak pointers to automatically collect values no longer in use
* Automatically delete keys for values no longer in use, removing the need to lock and loop the cache at various times
* Shrinks the underlying map when values are deleted
* Thread-safe

This uses a sharded map implementation based Josh Baker's excellent sharded map: https://github.com/tidwall/shardmap

The sharded map is updated to utilize generics and Go's maphash, which didn't exist when Josh made his.  This implementation beats sync.Map(even with the latest updates) for read/write in most cases by a very slight margin.

# Use

It is important to remember this isn't a TTL like cache, values stay in the cache until a value is no longer in use. Once that occurs, if a garbage collection occurs, the value is collected and its key removed from the map.

The following show the semantics for the cache.

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
cache.Set(id, d)
```

## Get a value from the cache
```go
d, ok := cache.Get(id)
if !ok {
	return fmt.Errorf("value not in cache")
}
```

## Delete a value form the cache
```go
// Old is the value you deleted, ok is true if the value was deleted vs just not found.
old, ok := cache.Del(id)
```
