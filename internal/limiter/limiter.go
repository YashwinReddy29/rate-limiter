package limiter

import (
	"context"
	"errors"
	"regexp"

	"github.com/YashwinReddy29/rate-limiter/internal/store"
)

var ErrInvalid = errors.New("client_id and resource must contain 1-128 letters, digits, dots, underscores or hyphens; cost must be 1-10000")
var identifier = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func Validate(clientID, resource string) error {
	if !identifier.MatchString(clientID) || !identifier.MatchString(resource) {
		return ErrInvalid
	}
	return nil
}

type Store interface {
	Window(context.Context, string, string, int64, int64, int64) (store.Decision, error)
	ResetKey(context.Context, string, string) error
	Ping(context.Context) error
}
type Result struct {
	Allowed      bool   `json:"allowed"`
	Remaining    int64  `json:"remaining"`
	Limit        int64  `json:"limit"`
	ResetAfterMs int64  `json:"reset_after_ms"`
	Reason       string `json:"reason"`
}
type RateLimiter struct{ store Store }

func New(s Store) *RateLimiter { return &RateLimiter{store: s} }
func (rl *RateLimiter) Check(ctx context.Context, clientID, resource string, cost int32) (*Result, error) {
	if err := Validate(clientID, resource); err != nil {
		return nil, err
	}
	if cost < 1 || cost > 10000 {
		return nil, ErrInvalid
	}
	q := GetQuota(clientID, resource)
	// An impossible request is rejected without accessing or allocating Redis state.
	if int64(cost) > q.Limit {
		return &Result{Limit: q.Limit, Reason: "cost exceeds quota limit"}, nil
	}
	d, err := rl.store.Window(ctx, clientID, resource, q.WindowSec, q.Limit, int64(cost))
	if err != nil {
		return nil, err
	}
	reason := "ok"
	if !d.Allowed {
		reason = "rate limit exceeded"
	}
	return &Result{Allowed: d.Allowed, Remaining: max(q.Limit-d.Used, 0), Limit: q.Limit, ResetAfterMs: d.ResetAfterMs, Reason: reason}, nil
}
func (rl *RateLimiter) Reset(ctx context.Context, clientID, resource string) error {
	if err := Validate(clientID, resource); err != nil {
		return err
	}
	return rl.store.ResetKey(ctx, clientID, resource)
}
func (rl *RateLimiter) Ready(ctx context.Context) error { return rl.store.Ping(ctx) }

type QuotaInfo struct {
	ClientID  string `json:"client_id"`
	Resource  string `json:"resource"`
	Limit     int64  `json:"limit"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
	WindowSec int64  `json:"window_sec"`
}

func (rl *RateLimiter) GetQuota(ctx context.Context, clientID, resource string) (*QuotaInfo, error) {
	if err := Validate(clientID, resource); err != nil {
		return nil, err
	}
	q := GetQuota(clientID, resource)
	d, err := rl.store.Window(ctx, clientID, resource, q.WindowSec, q.Limit, 0)
	if err != nil {
		return nil, err
	}
	return &QuotaInfo{ClientID: clientID, Resource: resource, Limit: q.Limit, Used: d.Used, Remaining: max(q.Limit-d.Used, 0), WindowSec: q.WindowSec}, nil
}
