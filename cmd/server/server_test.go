package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
	"github.com/YashwinReddy29/rate-limiter/internal/store"
	pb "github.com/YashwinReddy29/rate-limiter/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var testAuth = credentials{service: strings.Repeat("s", 32), admin: strings.Repeat("a", 32)}

type unavailableStore struct{}

func (unavailableStore) Window(context.Context, string, string, int64, int64, int64) (store.Decision, error) {
	return store.Decision{}, errors.New("private backend address")
}
func (unavailableStore) ResetKey(context.Context, string, string) error {
	return errors.New("private backend address")
}
func (unavailableStore) Ping(context.Context) error { return errors.New("private backend address") }
func TestHTTPGuardsAndFailure(t *testing.T) {
	h := (&httpServer{rl: limiter.New(unavailableStore{}), auth: testAuth, metrics: &metrics{}}).routes()
	cases := []struct {
		name, method, path, key, body string
		want                          int
	}{
		{"no auth", "POST", "/check", "", `{}`, 401},
		{"no admin", "POST", "/reset", testAuth.service, `{}`, 403},
		{"get reset", "GET", "/reset", testAuth.admin, ``, 405},
		{"invalid identifier", "POST", "/check", testAuth.service, `{"client_id":"a:b","resource":"api"}`, 400},
		{"null cost", "POST", "/check", testAuth.service, `{"client_id":"a","resource":"api","cost":null}`, 400},
		{"zero cost", "POST", "/check", testAuth.service, `{"client_id":"a","resource":"api","cost":0}`, 400},
		{"negative cost", "POST", "/check", testAuth.service, `{"client_id":"a","resource":"api","cost":-1}`, 400},
		{"huge cost", "POST", "/check", testAuth.service, `{"client_id":"a","resource":"api","cost":10001}`, 400},
		{"extra field", "POST", "/check", testAuth.service, `{"extra":1}`, 400},
		{"two objects", "POST", "/check", testAuth.service, `{} {}`, 400},
		{"oversized", "POST", "/check", testAuth.service, strings.Repeat(" ", 5000) + `{}`, 400},
		{"backend down", "POST", "/check", testAuth.service, `{"client_id":"a","resource":"api"}`, 503},
		{"ready down", "GET", "/ready", "", ``, 503},
		{"live", "GET", "/health", "", ``, 200},
		{"metrics forbidden", "GET", "/metrics", testAuth.service, ``, 403},
		{"metrics admin", "GET", "/metrics", testAuth.admin, ``, 200},
		{"no public benchmark", "GET", "/benchmark", testAuth.service, ``, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("X-API-Key", tc.key)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body)
			}
			if strings.Contains(w.Body.String(), "private backend") {
				t.Fatal("leaked backend error")
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request ID")
			}
		})
	}
}
func TestCredentials(t *testing.T) {
	if testAuth.validate() != nil {
		t.Fatal("valid rejected")
	}
	if (credentials{}).validate() == nil {
		t.Fatal("empty accepted")
	}
	if (credentials{service: testAuth.service, admin: testAuth.service}).validate() == nil {
		t.Fatal("same keys accepted")
	}
}
func TestGRPCAuthenticationAndErrors(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(testAuth.intercept))
	pb.RegisterRateLimiterServer(srv, &grpcServer{rl: limiter.New(unavailableStore{}), metrics: &metrics{}})
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	client := pb.NewRateLimiterClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = client.Check(ctx, &pb.CheckRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", testAuth.service)
	_, err = client.Reset(ctx, &pb.ResetRequest{ClientId: "a", Resource: "api"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	_, err = client.Check(ctx, &pb.CheckRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	_, err = client.Check(ctx, &pb.CheckRequest{ClientId: "a", Resource: "api", Cost: 1})
	if status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}
func TestHTTPRedisAdmission(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("REDIS_TEST_ADDR required")
	}
	rs := store.NewRedisStore(addr)
	defer rs.Close()
	rl := limiter.New(rs)
	id := "http-" + time.Now().Format("150405.000000000")
	defer rl.Reset(context.Background(), id, "auth")
	ts := httptest.NewServer((&httpServer{rl: rl, auth: testAuth, metrics: &metrics{}}).routes())
	defer ts.Close()
	for i := 0; i < 7; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/check", strings.NewReader(`{"client_id":"`+id+`","resource":"auth","cost":1}`))
		req.Header.Set("X-API-Key", testAuth.service)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		want := 200
		if i >= 5 {
			want = 429
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("missing retry header")
			}
		}
		if resp.StatusCode != want {
			t.Fatalf("request %d status %d want %d", i, resp.StatusCode, want)
		}
	}
	q, err := rl.GetQuota(context.Background(), id, "auth")
	if err != nil || q.Used != 5 {
		t.Fatalf("quota %+v %v", q, err)
	}
}
