package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
)

type httpServer struct {
	rl *limiter.RateLimiter
}

func (s *httpServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/check",     s.handleCheck)
	mux.HandleFunc("/reset",     s.handleReset)
	mux.HandleFunc("/quota",     s.handleQuota)
	mux.HandleFunc("/benchmark", s.handleBenchmark)
	mux.HandleFunc("/health",    s.handleHealth)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *httpServer) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var req struct {
		ClientID string `json:"client_id"`
		Resource string `json:"resource"`
		Cost     int32  `json:"cost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if req.Cost == 0 {
		req.Cost = 1
	}
	result, err := s.rl.Check(r.Context(), req.ClientID, req.Resource, req.Cost)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if !result.Allowed {
		status = http.StatusTooManyRequests
	}
	writeJSON(w, status, result)
}

func (s *httpServer) handleReset(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	resource := r.URL.Query().Get("resource")
	err := s.rl.Reset(r.Context(), clientID, resource)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}

func (s *httpServer) handleQuota(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	resource := r.URL.Query().Get("resource")
	info, err := s.rl.GetQuota(r.Context(), clientID, resource)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, info)
}

func (s *httpServer) handleBenchmark(w http.ResponseWriter, r *http.Request) {
	nStr := r.URL.Query().Get("n")
	n := 10000
	if nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil {
			n = parsed
		}
	}
	result := s.rl.BenchmarkLatency(r.Context(), n)
	writeJSON(w, 200, result)
}

func (s *httpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}