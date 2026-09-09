package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testStore(t *testing.T) *RedisStore {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR for real Redis integration tests")
	}
	s := NewRedisStore(addr)
	t.Cleanup(func() { s.Close() })
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestConcurrentAdmission(t *testing.T) {
	s := testStore(t)
	other := testStore(t)
	ctx := context.Background()
	id := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
	t.Cleanup(func() { s.ResetKey(ctx, id, "api") })
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chosen := s
			if i%2 == 0 {
				chosen = other
			}
			d, err := chosen.Window(ctx, id, "api", 60, 50, 1)
			if err != nil {
				t.Error(err)
				return
			}
			if d.Allowed {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 50 {
		t.Fatalf("allowed %d, want exactly 50", allowed.Load())
	}
	d, err := s.Window(ctx, id, "api", 60, 50, 0)
	if err != nil || d.Used != 50 {
		t.Fatalf("used %+v err %v", d, err)
	}
}
func TestWeightedDenialAndExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id := fmt.Sprintf("weighted-%d", time.Now().UnixNano())
	t.Cleanup(func() { s.ResetKey(ctx, id, "api") })
	d, err := s.Window(ctx, id, "api", 1, 5, 3)
	if err != nil || !d.Allowed || d.Used != 3 {
		t.Fatalf("first %+v %v", d, err)
	}
	d, err = s.Window(ctx, id, "api", 1, 5, 3)
	if err != nil || d.Allowed || d.Used != 3 {
		t.Fatalf("denial consumed quota: %+v %v", d, err)
	}
	d, err = s.Window(ctx, id, "api", 1, 5, 2)
	if err != nil || !d.Allowed || d.Used != 5 || d.ResetAfterMs > 1000 || d.ResetAfterMs <= 0 {
		t.Fatalf("exact limit %+v %v", d, err)
	}
	time.Sleep(1100 * time.Millisecond)
	d, err = s.Window(ctx, id, "api", 1, 5, 5)
	if err != nil || !d.Allowed || d.Used != 5 {
		t.Fatalf("expiry %+v %v", d, err)
	}
	if err = s.ResetKey(ctx, id, "api"); err != nil {
		t.Fatal(err)
	}
	d, err = s.Window(ctx, id, "api", 1, 5, 0)
	if err != nil || d.Used != 0 || d.ResetAfterMs != 0 {
		t.Fatalf("reset %+v %v", d, err)
	}
}
func TestTenantIsolation(t *testing.T) {
	if key("a:b", "c") == key("a", "b:c") {
		t.Fatal("ambiguous key encoding")
	}
	s := testStore(t)
	ctx := context.Background()
	id := fmt.Sprintf("isolation-%d", time.Now().UnixNano())
	for _, resource := range []string{"api", "search"} {
		t.Cleanup(func() { s.ResetKey(ctx, id, resource) })
		d, err := s.Window(ctx, id, resource, 60, 1, 1)
		if err != nil || !d.Allowed {
			t.Fatalf("resource isolation %+v %v", d, err)
		}
	}
}
