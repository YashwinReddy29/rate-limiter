package limiter

import (
	"context"
	"github.com/YashwinReddy29/rate-limiter/internal/store"
	"testing"
)

type recordingStore struct{ calls int }

func (s *recordingStore) Window(context.Context, string, string, int64, int64, int64) (store.Decision, error) {
	s.calls++
	return store.Decision{Allowed: true, Used: 1}, nil
}
func (s *recordingStore) ResetKey(context.Context, string, string) error { s.calls++; return nil }
func (s *recordingStore) Ping(context.Context) error                     { return nil }
func TestInvalidAndImpossibleCostsDoNotReachStore(t *testing.T) {
	s := &recordingStore{}
	rl := New(s)
	ctx := context.Background()
	for _, cost := range []int32{-1, 0, 10001} {
		if _, err := rl.Check(ctx, "a", "api", cost); err != ErrInvalid {
			t.Fatal(err)
		}
	}
	r, err := rl.Check(ctx, "a", "auth", 6)
	if err != nil || r.Allowed {
		t.Fatalf("%+v %v", r, err)
	}
	if s.calls != 0 {
		t.Fatal("unnecessary state allocation")
	}
}
func TestQuotaSelection(t *testing.T) {
	for _, tc := range []struct {
		client, resource string
		want             int64
	}{{"free_client", "api", 100}, {"premium_client", "api", 10000}, {"unknown", "search", 50}, {"unknown", "custom", 100}} {
		if got := GetQuota(tc.client, tc.resource); got.Limit != tc.want {
			t.Fatal(got)
		}
	}
}
func TestIdentifierValidation(t *testing.T) {
	for _, v := range []string{"", "a:b", "a\x00b", "a b"} {
		if Validate(v, "api") != ErrInvalid {
			t.Fatalf("accepted %q", v)
		}
	}
	if Validate("client-1.foo", "api_v2") != nil {
		t.Fatal("valid rejected")
	}
}
