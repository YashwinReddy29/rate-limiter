package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
)

type httpServer struct {
	rl      *limiter.RateLimiter
	auth    credentials
	metrics *metrics
}

func (s *httpServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /check", s.handleCheck)
	mux.HandleFunc("POST /reset", s.handleReset)
	mux.HandleFunc("GET /quota", s.handleQuota)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := s.rl.Ready(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"error": "dependency unavailable"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /metrics", s.metrics.serve)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		var id [16]byte
		_, _ = rand.Read(id[:])
		requestID := hex.EncodeToString(id[:])
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		rw := &responseWriter{ResponseWriter: w, status: 200}
		defer func() {
			s.metrics.requests.Add(1)
			slog.Info("http_request", "request_id", requestID, "method", r.Method, "status", rw.status, "duration_ms", time.Since(started).Milliseconds())
		}()
		if r.URL.Path != "/health" && r.URL.Path != "/ready" {
			key := r.Header.Get("X-API-Key")
			if !s.auth.authorized(key, false) {
				writeJSON(rw, 401, map[string]string{"error": "authentication required"})
				return
			}
			if (r.URL.Path == "/reset" || r.URL.Path == "/metrics") && !s.auth.authorized(key, true) {
				writeJSON(rw, 403, map[string]string{"error": "administrator required"})
				return
			}
		}
		mux.ServeHTTP(rw, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func httpError(w http.ResponseWriter, err error) {
	if errors.Is(err, limiter.ErrInvalid) {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 503, map[string]string{"error": "rate limiter unavailable"})
}
func (s *httpServer) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string          `json:"client_id"`
		Resource string          `json:"resource"`
		Cost     json.RawMessage `json:"cost"`
	}
	if err := decode(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON request"})
		return
	}
	cost := int32(1)
	if len(req.Cost) > 0 {
		if string(req.Cost) == "null" || json.Unmarshal(req.Cost, &cost) != nil {
			writeJSON(w, 400, map[string]string{"error": "cost must be an integer"})
			return
		}
	}
	start := time.Now()
	result, err := s.rl.Check(r.Context(), req.ClientID, req.Resource, cost)
	s.metrics.observe(result, err, time.Since(start))
	if err != nil {
		httpError(w, err)
		return
	}
	status := 200
	if !result.Allowed {
		status = 429
		if result.ResetAfterMs > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt((result.ResetAfterMs+999)/1000, 10))
		}
	}
	w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
	writeJSON(w, status, result)
}
func (s *httpServer) handleReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		Resource string `json:"resource"`
	}
	if err := decode(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON request"})
		return
	}
	if err := s.rl.Reset(r.Context(), req.ClientID, req.Resource); err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (s *httpServer) handleQuota(w http.ResponseWriter, r *http.Request) {
	info, err := s.rl.GetQuota(r.Context(), r.URL.Query().Get("client_id"), r.URL.Query().Get("resource"))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, info)
}
