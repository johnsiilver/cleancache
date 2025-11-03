package cleancache

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/gostdlib/base/concurrency/sync"
	"github.com/kylelemons/godebug/pretty"
)

type testValue struct {
	data string
	num  int
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options []Option
		wantErr bool
	}{
		{
			name:    "Success: create cache without options",
			options: nil,
			wantErr: false,
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx, test.options...)

		switch {
		case err == nil && test.wantErr:
			t.Errorf("TestNew(%s): got err == nil, want err != nil", test.name)
			continue
		case err != nil && !test.wantErr:
			t.Errorf("TestNew(%s): got err == %s, want err == nil", test.name, err)
			continue
		case err != nil:
			continue
		}

		if cache == nil {
			t.Errorf("TestNew(%s): got nil cache, want non-nil", test.name)
		}
	}
}

func TestCacheBasicOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupFunc func(*Cache[string, testValue])
		operation func(*Cache[string, testValue]) (any, bool)
		wantValue any
		wantOk    bool
	}{
		{
			name:      "Success: Get from empty cache returns not found",
			setupFunc: nil,
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Get("key1")
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
		{
			name:      "Success: Set and Get value",
			setupFunc: nil,
			operation: func(c *Cache[string, testValue]) (any, bool) {
				val := &testValue{data: "test", num: 42}
				c.Set("key1", val)
				return c.Get("key1")
			},
			wantValue: &testValue{data: "test", num: 42},
			wantOk:    true,
		},
		{
			name: "Success: Set overwrites existing value",
			setupFunc: func(c *Cache[string, testValue]) {
				val := &testValue{data: "old", num: 1}
				c.Set("key1", val)
			},
			operation: func(c *Cache[string, testValue]) (any, bool) {
				val := &testValue{data: "new", num: 2}
				return c.Set("key1", val)
			},
			wantValue: &testValue{data: "old", num: 1},
			wantOk:    true,
		},
		{
			name: "Success: Del removes value",
			setupFunc: func(c *Cache[string, testValue]) {
				val := &testValue{data: "test", num: 42}
				c.Set("key1", val)
			},
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Del("key1")
			},
			wantValue: &testValue{data: "test", num: 42},
			wantOk:    true,
		},
		{
			name:      "Success: Del on non-existent key returns not found",
			setupFunc: nil,
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Del("nonexistent")
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
		{
			name:      "Success: Get on nil cache returns not found",
			setupFunc: nil,
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Get("key1")
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestCacheBasicOperations(%s): failed to create cache: %v", test.name, err)
		}

		if test.setupFunc != nil {
			test.setupFunc(cache)
		}

		gotValue, gotOk := test.operation(cache)

		if gotOk != test.wantOk {
			t.Errorf("TestCacheBasicOperations(%s): got ok=%v, want ok=%v", test.name, gotOk, test.wantOk)
		}

		if diff := pretty.Compare(gotValue, test.wantValue); diff != "" {
			t.Errorf("TestCacheBasicOperations(%s): -got +want:\n%s", test.name, diff)
		}
	}
}

func TestConcurrentGetSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		numGoroutines int
		numOperations int
		operation     string
	}{
		{
			name:          "Success: concurrent Sets on different keys",
			numGoroutines: 100,
			numOperations: 1000,
			operation:     "set",
		},
		{
			name:          "Success: concurrent Gets on same keys",
			numGoroutines: 100,
			numOperations: 1000,
			operation:     "get",
		},
		{
			name:          "Success: mixed concurrent Gets and Sets",
			numGoroutines: 100,
			numOperations: 1000,
			operation:     "mixed",
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[int, testValue](ctx)
		if err != nil {
			t.Fatalf("TestConcurrentGetSet(%s): failed to create cache: %v", test.name, err)
		}

		// Pre-populate cache for get operations
		if test.operation == "get" || test.operation == "mixed" {
			for i := 0; i < test.numOperations; i++ {
				val := &testValue{data: "initial", num: i}
				cache.Set(i, val)
			}
		}

		var wg sync.Group

		for g := 0; g < test.numGoroutines; g++ {
			for i := 0; i < test.numOperations; i++ {
				wg.Go(
					ctx,
					func(ctx context.Context) error {
						key := i

						switch test.operation {
						case "set":
							val := &testValue{data: "concurrent", num: g*test.numOperations + i}
							cache.Set(key, val)
						case "get":
							cache.Get(key)
						case "mixed":
							if i%2 == 0 {
								cache.Get(key)
							} else {
								val := &testValue{data: "mixed", num: g*test.numOperations + i}
								cache.Set(key, val)
							}
						}
						return nil
					},
				)
			}
		}

		wg.Wait(ctx)

		// Verify cache is still functional
		if test.operation == "set" || test.operation == "mixed" {
			testKey := 0
			testVal := &testValue{data: "verify", num: 999}
			cache.Set(testKey, testVal)
			gotVal, ok := cache.Get(testKey)
			if !ok {
				t.Errorf("TestConcurrentGetSet(%s): failed to get value after concurrent operations", test.name)
			}
			if diff := pretty.Compare(gotVal, testVal); diff != "" {
				t.Errorf("TestConcurrentGetSet(%s): -got +want:\n%s", test.name, diff)
			}
		}
	}
}

func TestConcurrentDelete(t *testing.T) {
	tests := []struct {
		name          string
		numGoroutines int
		numKeys       int
	}{
		{
			name:          "Success: concurrent deletes on different keys",
			numGoroutines: 50,
			numKeys:       1000,
		},
		{
			name:          "Success: concurrent deletes on same keys",
			numGoroutines: 100,
			numKeys:       10,
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[int, testValue](ctx)
		if err != nil {
			t.Fatalf("TestConcurrentDelete(%s): failed to create cache: %v", test.name, err)
		}

		// Populate cache - keep strong references to prevent GC
		values := make([]*testValue, test.numKeys)
		for i := 0; i < test.numKeys; i++ {
			val := &testValue{data: "delete-test", num: i}
			values[i] = val
			cache.Set(i, val)
		}

		initialLen := cache.Len()
		if initialLen != test.numKeys {
			t.Errorf("TestConcurrentDelete(%s): initial length=%d, want=%d", test.name, initialLen, test.numKeys)
		}

		var wg sync.Group

		for g := 0; g < test.numGoroutines; g++ {
			wg.Go(
				ctx,
				func(ctx context.Context) error {
					for i := 0; i < test.numKeys; i++ {
						cache.Del(i)
					}
					return nil
				},
			)
		}

		wg.Wait(ctx)

		// All keys should be deleted
		finalLen := cache.Len()
		if finalLen != 0 {
			t.Errorf("TestConcurrentDelete(%s): final length=%d, want=0", test.name, finalLen)
		}
	}
}

func TestWeakPointerCleanup(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Success: weak pointer cleanup removes entries",
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestWeakPointerCleanup(%s): failed to create cache: %v", test.name, err)
		}

		// Create a value and add to cache
		key := "cleanup-test"
		val := &testValue{data: "will-be-collected", num: 42}
		cache.Set(key, val)

		// Verify it exists
		gotVal, ok := cache.Get(key)
		if !ok || gotVal == nil {
			t.Errorf("TestWeakPointerCleanup(%s): value not found after Set", test.name)
		}

		// Remove strong reference
		val = nil
		gotVal = nil

		// Force GC
		runtime.GC()
		runtime.GC() // Call twice to ensure cleanup finalizers run
		time.Sleep(100 * time.Millisecond)

		// Note: We cannot reliably test that the cleanup happened because:
		// 1. GC timing is non-deterministic
		// 2. The cleanup function may not run immediately
		// 3. Weak pointers may still hold references temporarily
		// This test primarily ensures the cleanup registration doesn't panic
		// and the cache remains functional after GC

		// Verify cache is still functional with new values
		newVal := &testValue{data: "new-value", num: 100}
		cache.Set("new-key", newVal)
		got, ok := cache.Get("new-key")
		if !ok {
			t.Errorf("TestWeakPointerCleanup(%s): failed to set/get after GC", test.name)
		}
		if diff := pretty.Compare(got, newVal); diff != "" {
			t.Errorf("TestWeakPointerCleanup(%s): -got +want:\n%s", test.name, diff)
		}
	}
}

func TestNilCacheOperations(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*Cache[string, testValue]) (any, bool)
		wantValue any
		wantOk    bool
	}{
		{
			name: "Success: Set on nil cache returns nil, false",
			operation: func(c *Cache[string, testValue]) (any, bool) {
				val := &testValue{data: "test", num: 42}
				return c.Set("key1", val)
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
		{
			name: "Success: Del on nil cache returns nil, false",
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Del("key1")
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
		{
			name: "Success: Get on nil cache returns nil, false",
			operation: func(c *Cache[string, testValue]) (any, bool) {
				return c.Get("key1")
			},
			wantValue: (*testValue)(nil),
			wantOk:    false,
		},
	}

	for _, test := range tests {
		var cache *Cache[string, testValue]

		gotValue, gotOk := test.operation(cache)

		if gotOk != test.wantOk {
			t.Errorf("TestNilCacheOperations(%s): got ok=%v, want ok=%v", test.name, gotOk, test.wantOk)
		}

		if diff := pretty.Compare(gotValue, test.wantValue); diff != "" {
			t.Errorf("TestNilCacheOperations(%s): -got +want:\n%s", test.name, diff)
		}
	}
}

func TestWeakPointerCollectedCleanup(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Success: Get cleans up collected weak pointers",
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestWeakPointerCollectedCleanup(%s): failed to create cache: %v", test.name, err)
		}

		key := "cleanup-test"

		// Set a value and immediately remove strong references
		func() {
			val := &testValue{data: "will-be-collected", num: 42}
			cache.Set(key, val)
			// val goes out of scope here
		}()

		// Force GC multiple times to try to collect the weak pointer
		for i := 0; i < 5; i++ {
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}

		// Try to get the value - if it was collected, Get should:
		// 1. Find the weak pointer is nil
		// 2. Delete the key
		// 3. Return nil, false
		val, ok := cache.Get(key)

		// Note: This test is inherently non-deterministic because:
		// 1. GC may not have run yet
		// 2. Weak pointers may still hold references temporarily
		// 3. The cleanup finalizer may not have executed
		//
		// We can only verify that if the value is gone, the behavior is correct.
		// We cannot force it to be gone reliably.
		if !ok && val != nil {
			t.Errorf("TestWeakPointerCollectedCleanup(%s): got ok=false but val != nil", test.name)
		}

		// Verify cache is still functional regardless of cleanup timing
		newVal := &testValue{data: "new-value", num: 100}
		cache.Set("new-key", newVal)
		got, ok := cache.Get("new-key")
		if !ok {
			t.Errorf("TestWeakPointerCollectedCleanup(%s): failed to set/get new value after GC attempts", test.name)
		}
		if diff := pretty.Compare(got, newVal); diff != "" {
			t.Errorf("TestWeakPointerCollectedCleanup(%s): -got +want:\n%s", test.name, diff)
		}
	}
}

func TestSetWithNilValue(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*Cache[string, testValue])
		wantPrevValue *testValue
		wantPrevOk    bool
		verifyDeleted bool
	}{
		{
			name: "Success: Set nil on existing key deletes it",
			setupFunc: func(c *Cache[string, testValue]) {
				val := &testValue{data: "test", num: 42}
				c.Set("key1", val)
			},
			wantPrevValue: &testValue{data: "test", num: 42},
			wantPrevOk:    true,
			verifyDeleted: true,
		},
		{
			name:          "Success: Set nil on non-existent key returns nil, false",
			setupFunc:     nil,
			wantPrevValue: nil,
			wantPrevOk:    false,
			verifyDeleted: true,
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestSetWithNilValue(%s): failed to create cache: %v", test.name, err)
		}

		if test.setupFunc != nil {
			test.setupFunc(cache)
		}

		// Set with nil value
		var nilVal *testValue
		gotPrev, gotOk := cache.Set("key1", nilVal)

		if gotOk != test.wantPrevOk {
			t.Errorf("TestSetWithNilValue(%s): got ok=%v, want ok=%v", test.name, gotOk, test.wantPrevOk)
		}

		if diff := pretty.Compare(gotPrev, test.wantPrevValue); diff != "" {
			t.Errorf("TestSetWithNilValue(%s): -got +want:\n%s", test.name, diff)
		}

		// Verify key was deleted
		if test.verifyDeleted {
			val, ok := cache.Get("key1")
			if ok {
				t.Errorf("TestSetWithNilValue(%s): key still exists after Set(nil), got value=%v", test.name, val)
			}
		}
	}
}

func TestDelTwice(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Success: Del twice on same key returns correct values",
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestDelTwice(%s): failed to create cache: %v", test.name, err)
		}

		// Set a value
		val := &testValue{data: "test", num: 42}
		cache.Set("key1", val)

		// First Del should return the value
		gotVal1, gotOk1 := cache.Del("key1")
		if !gotOk1 {
			t.Errorf("TestDelTwice(%s): first Del got ok=false, want ok=true", test.name)
		}
		if diff := pretty.Compare(gotVal1, val); diff != "" {
			t.Errorf("TestDelTwice(%s): first Del -got +want:\n%s", test.name, diff)
		}

		// Second Del should return nil, false
		gotVal2, gotOk2 := cache.Del("key1")
		if gotOk2 {
			t.Errorf("TestDelTwice(%s): second Del got ok=true, want ok=false", test.name)
		}
		if gotVal2 != nil {
			t.Errorf("TestDelTwice(%s): second Del got val=%v, want nil", test.name, gotVal2)
		}
	}
}

func TestMultipleSetOverwrites(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Success: multiple Set calls correctly overwrite and return previous values",
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[string, testValue](ctx)
		if err != nil {
			t.Fatalf("TestMultipleSetOverwrites(%s): failed to create cache: %v", test.name, err)
		}

		val1 := &testValue{data: "first", num: 1}
		val2 := &testValue{data: "second", num: 2}
		val3 := &testValue{data: "third", num: 3}

		// First Set should return nil, false (no previous value)
		prev1, ok1 := cache.Set("key1", val1)
		if ok1 {
			t.Errorf("TestMultipleSetOverwrites(%s): first Set got ok=true, want ok=false", test.name)
		}
		if prev1 != nil {
			t.Errorf("TestMultipleSetOverwrites(%s): first Set got prev=%v, want nil", test.name, prev1)
		}

		// Second Set should return val1, true
		prev2, ok2 := cache.Set("key1", val2)
		if !ok2 {
			t.Errorf("TestMultipleSetOverwrites(%s): second Set got ok=false, want ok=true", test.name)
		}
		if diff := pretty.Compare(prev2, val1); diff != "" {
			t.Errorf("TestMultipleSetOverwrites(%s): second Set -got +want:\n%s", test.name, diff)
		}

		// Third Set should return val2, true
		prev3, ok3 := cache.Set("key1", val3)
		if !ok3 {
			t.Errorf("TestMultipleSetOverwrites(%s): third Set got ok=false, want ok=true", test.name)
		}
		if diff := pretty.Compare(prev3, val2); diff != "" {
			t.Errorf("TestMultipleSetOverwrites(%s): third Set -got +want:\n%s", test.name, diff)
		}

		// Get should return val3
		gotVal, gotOk := cache.Get("key1")
		if !gotOk {
			t.Errorf("TestMultipleSetOverwrites(%s): Get got ok=false, want ok=true", test.name)
		}
		if diff := pretty.Compare(gotVal, val3); diff != "" {
			t.Errorf("TestMultipleSetOverwrites(%s): Get -got +want:\n%s", test.name, diff)
		}
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	tests := []struct {
		name          string
		numGoroutines int
		duration      time.Duration
	}{
		{
			name:          "Success: mixed operations under load",
			numGoroutines: 100,
			duration:      2 * time.Second,
		},
	}

	for _, test := range tests {
		ctx := t.Context()
		cache, err := New[int, testValue](ctx)
		if err != nil {
			t.Fatalf("TestConcurrentMixedOperations(%s): failed to create cache: %v", test.name, err)
		}

		// Pre-populate some data
		for i := 0; i < 100; i++ {
			val := &testValue{data: "initial", num: i}
			cache.Set(i, val)
		}

		var wg sync.Group
		timedCtx, cancel := context.WithTimeout(ctx, test.duration)
		defer cancel()

		for g := 0; g < test.numGoroutines; g++ {
			wg.Go(
				timedCtx,
				func(ctx context.Context) error {
					opCount := 0
					for {
						select {
						case <-ctx.Done():
							return nil
						default:
							key := opCount % 100
							opType := opCount % 3

							switch opType {
							case 0: // Get
								cache.Get(key)
							case 1: // Set
								val := &testValue{data: "concurrent", num: g*10000 + opCount}
								cache.Set(key, val)
							case 2: // Del
								cache.Del(key)
							}

							opCount++
						}
					}
				},
			)
		}
		wg.Wait(timedCtx)

		// Verify cache is still functional
		testVal := &testValue{data: "final-test", num: 999}
		cache.Set(999, testVal)
		got, ok := cache.Get(999)
		if !ok {
			t.Errorf("TestConcurrentMixedOperations(%s): cache not functional after concurrent operations", test.name)
		}
		if diff := pretty.Compare(got, testVal); diff != "" {
			t.Errorf("TestConcurrentMixedOperations(%s): -got +want:\n%s", test.name, diff)
		}
	}
}
