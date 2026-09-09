package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
)

type metrics struct{ requests, allowed, denied, failed, nanos atomic.Uint64 }

func (m *metrics) observe(r *limiter.Result, err error, d time.Duration) {
	m.nanos.Add(uint64(d))
	if err != nil {
		m.failed.Add(1)
	} else if r.Allowed {
		m.allowed.Add(1)
	} else {
		m.denied.Add(1)
	}
}
func (m *metrics) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	a, d, f := m.allowed.Load(), m.denied.Load(), m.failed.Load()
	fmt.Fprintf(w, "# TYPE ratelimiter_http_requests_total counter\nratelimiter_http_requests_total %d\n# TYPE ratelimiter_decisions_total counter\nratelimiter_decisions_total{outcome=\"allowed\"} %d\nratelimiter_decisions_total{outcome=\"denied\"} %d\nratelimiter_decisions_total{outcome=\"error\"} %d\n# TYPE ratelimiter_decision_duration_seconds summary\nratelimiter_decision_duration_seconds_sum %g\nratelimiter_decision_duration_seconds_count %d\n", m.requests.Load(), a, d, f, float64(m.nanos.Load())/1e9, a+d+f)
}
