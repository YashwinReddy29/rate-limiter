package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/YashwinReddy29/rate-limiter/internal/store"
)

type Result struct {
	Allowed      bool
	Remaining    int64
	Limit        int64
	ResetAfterMs int64
	Reason       string
}

type RateLimiter struct {
	store *store.RedisStore
}

func New(s *store.RedisStore) *RateLimiter {
	return &RateLimiter{store: s}
}

func (rl *RateLimiter) Check(ctx context.Context, clientID, resource string, cost int32) (*Result, error) {
	quota := GetQuota(clientID, resource)
	if cost <= 0 {
		cost = 1
	}

	count, err := rl.store.SlidingWindowIncr(ctx, clientID, resource, quota.WindowSec, int64(cost))
	if err != nil {
		return nil, fmt.Errorf("redis error: %w", err)
	}

	remaining := quota.Limit - count
	allowed := count <= quota.Limit
	resetAfterMs := quota.WindowSec * 1000

	reason := "ok"
	if !allowed {
		remaining = 0
		reason = fmt.Sprintf("rate limit exceeded: %d/%d requests in %ds window",
			count, quota.Limit, quota.WindowSec)
	}

	return &Result{
		Allowed:      allowed,
		Remaining:    max(remaining, 0),
		Limit:        quota.Limit,
		ResetAfterMs: resetAfterMs,
		Reason:       reason,
	}, nil
}

func (rl *RateLimiter) Reset(ctx context.Context, clientID, resource string) error {
	return rl.store.ResetKey(ctx, clientID, resource)
}

func (rl *RateLimiter) GetQuota(ctx context.Context, clientID, resource string) (*QuotaInfo, error) {
	quota := GetQuota(clientID, resource)
	used, err := rl.store.SlidingWindowCount(ctx, clientID, resource, quota.WindowSec)
	if err != nil {
		return nil, err
	}
	return &QuotaInfo{
		ClientID:  clientID,
		Resource:  resource,
		Limit:     quota.Limit,
		Used:      used,
		Remaining: max(quota.Limit-used, 0),
		WindowSec: quota.WindowSec,
	}, nil
}

type QuotaInfo struct {
	ClientID  string
	Resource  string
	Limit     int64
	Used      int64
	Remaining int64
	WindowSec int64
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// BenchmarkLatency measures p99 scoring latency
func (rl *RateLimiter) BenchmarkLatency(ctx context.Context, n int) map[string]float64 {
	latencies := make([]float64, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		rl.Check(ctx, "bench_client", "api", 1)
		latencies[i] = float64(time.Since(start).Microseconds()) / 1000.0
	}
	// p50, p99
	sum := 0.0
	for _, v := range latencies {
		sum += v
	}
	avg := sum / float64(n)

	// Simple p99 (sort-free approximation)
	p99 := latencies[int(float64(n)*0.99)]
	return map[string]float64{"avg_ms": avg, "p99_ms": p99, "n": float64(n)}
}