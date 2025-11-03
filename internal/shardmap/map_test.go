// Copyright 2019 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an ISC-style
// license that can be found in the LICENSE file.

package shardmap

import (
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"testing"
	"time"
)

type keyT = string

func k(key int) keyT {
	return strconv.FormatInt(int64(key), 10)
}

func add(x keyT, delta int) int {
	i, err := strconv.ParseInt(x, 10, 64)
	if err != nil {
		panic(err)
	}
	return int(i + int64(delta))
}

// /////////////////////////
func random(N int, perm bool) []keyT {
	nums := make([]keyT, N)
	if perm {
		for i, x := range rand.Perm(N) {
			nums[i] = k(x)
		}
	} else {
		m := make(map[keyT]bool)
		for len(m) < N {
			m[k(int(rand.Uint64()))] = true
		}
		var i int
		for k := range m {
			nums[i] = k
			i++
		}
	}
	return nums
}

func shuffle(nums []keyT) {
	for i := range nums {
		j := rand.Intn(i + 1)
		nums[i], nums[j] = nums[j], nums[i]
	}
}

func TestRandomData(t *testing.T) {
	N := 10000
	start := time.Now()
	for time.Since(start) < time.Second*2 {
		nums := random(N, true)
		m := New[string, string](N / ((rand.Int() % 3) + 1))

		// Keep strong references to prevent GC
		strongRefs := make(map[string]*string)

		v, ok := m.Get(k(999))
		if ok || v != nil {
			t.Fatalf("expected %v, got %v", nil, v)
		}
		v, ok = m.Delete(k(999))
		if ok || v != nil {
			t.Fatalf("expected %v, got %v", nil, v)
		}
		if m.Len() != 0 {
			t.Fatalf("expected %v, got %v", 0, m.Len())
		}
		// set a bunch of items
		for i := 0; i < len(nums); i++ {
			// Create a new heap-allocated string
			val := nums[i]
			ptr := &val
			strongRefs[nums[i]] = ptr
			v, ok := m.Set(nums[i], ptr)
			if ok || v != nil {
				t.Fatalf("expected %v, got %v", nil, v)
			}
		}
		if m.Len() != N {
			t.Fatalf("expected %v, got %v", N, m.Len())
		}
		// retrieve all the items
		shuffle(nums)
		for i := 0; i < len(nums); i++ {
			v, ok := m.Get(nums[i])
			if !ok || *v == "" || *v != nums[i] {
				t.Fatalf("expected %v, got %v", nums[i], *v)
			}
		}
		// replace all the items
		shuffle(nums)
		for i := 0; i < len(nums); i++ {
			// Create a new heap-allocated string
			ptr := new(string)
			*ptr = strconv.Itoa(add(nums[i], 1))
			strongRefs[nums[i]] = ptr
			v, ok := m.Set(nums[i], ptr)
			if !ok || *v != nums[i] {
				t.Fatalf("expected %v, got %v", nums[i], v)
			}
		}
		if m.Len() != N {
			t.Fatalf("expected %v, got %v", N, m.Len())
		}
		// retrieve all the items
		shuffle(nums)
		for i := 0; i < len(nums); i++ {
			v, ok := m.Get(nums[i])
			want := add(nums[i], 1)
			wantStr := strconv.Itoa(want)
			if !ok || *v != wantStr {
				t.Fatalf("expected %v, got %v", add(nums[i], 1), v)
			}
		}
		// remove half the items
		shuffle(nums)
		for i := 0; i < len(nums)/2; i++ {
			v, ok := m.Delete(nums[i])
			want := add(nums[i], 1)
			wantStr := strconv.Itoa(want)
			if !ok || *v != wantStr {
				t.Fatalf("expected %v, got %v", add(nums[i], 1), v)
			}
		}
		if m.Len() != N/2 {
			t.Fatalf("expected %v, got %v", N/2, m.Len())
		}
		// check to make sure that the items have been removed
		for i := 0; i < len(nums)/2; i++ {
			v, ok := m.Get(nums[i])
			if ok || v != nil {
				t.Fatalf("expected %v, got %v", nil, v)
			}
		}
		// check the second half of the items
		for i := len(nums) / 2; i < len(nums); i++ {
			v, ok := m.Get(nums[i])
			want := add(nums[i], 1)
			wantStr := strconv.Itoa(want)
			if !ok || *v != wantStr {
				t.Fatalf("expected %v, got %v", add(nums[i], 1), v)
			}
		}
		// try to delete again, make sure they don't exist
		for i := 0; i < len(nums)/2; i++ {
			v, ok := m.Delete(nums[i])
			if ok || v != nil {
				t.Fatalf("expected %v, got %v", nil, v)
			}
		}
		if m.Len() != N/2 {
			t.Fatalf("expected %v, got %v", N/2, m.Len())
		}
		for k, v := range m.all() {
			i := add(k, 1)
			str := strconv.Itoa(i)
			if *v != str {
				t.Fatalf("expected %v, got %v", add(k, 1), v)
			}
		}
		var n int
		for range m.all() {
			n++
			break
		}

		if n != 1 {
			t.Fatalf("expected %v, got %v", 1, n)
		}
		for i := len(nums) / 2; i < len(nums); i++ {
			v, ok := m.Delete(nums[i])
			val := add(nums[i], 1)
			valStr := strconv.Itoa(val)
			if !ok || *v != valStr {
				t.Fatalf("expected %v, got %v", add(nums[i], 1), v)
			}
		}
		// Keep strong references alive until the end of the iteration
		runtime.KeepAlive(strongRefs)
	}
}

func TestClear(t *testing.T) {
	var m Map[string, int]
	// Keep strong references to prevent GC
	strongRefs := make([]*int, 1000)
	for i := 0; i < 1000; i++ {
		// Create a new heap-allocated int
		val := i
		ptr := &val
		strongRefs[i] = ptr
		m.Set(fmt.Sprintf("%d", i), ptr)
	}
	if m.Len() != 1000 {
		t.Fatalf("expected '%v', got '%v'", 1000, m.Len())
	}
	m.Clear()
	if m.Len() != 0 {
		t.Fatalf("expected '%v', got '%v'", 0, m.Len())
	}
	// Keep strong references alive until the end
	runtime.KeepAlive(strongRefs)
}
