// Standalone HTTP load generator. Never runs within the service process.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/YashwinReddy29/rate-limiter/internal/loadtest"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

func main() {
	endpoint := flag.String("url", "http://127.0.0.1:8080", "service URL")
	n := flag.Int("n", 10000, "measured requests (1-1000000)")
	workers := flag.Int("concurrency", 50, "workers (1-1000)")
	mode := flag.String("mode", "shared", "shared quota or unique clients")
	flag.Parse()
	if *n < 1 || *n > 1000000 || *workers < 1 || *workers > 1000 || (*mode != "shared" && *mode != "unique") || os.Getenv("API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "invalid flags or missing API_KEY")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{MaxIdleConns: *workers, MaxIdleConnsPerHost: *workers}}
	defer client.CloseIdleConnections()
	prefix := fmt.Sprintf("bench-%d", time.Now().UnixNano())
	samples := make([]float64, *n)
	outcomes := make([]int, *n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				id := prefix
				if *mode == "unique" {
					id = fmt.Sprintf("%s-%d", prefix, i)
				}
				payload, _ := json.Marshal(map[string]any{"client_id": id, "resource": "api", "cost": 1})
				req, err := http.NewRequest(http.MethodPost, *endpoint+"/check", bytes.NewReader(payload))
				if err != nil {
					outcomes[i] = -1
					continue
				}
				req.Header.Set("X-API-Key", os.Getenv("API_KEY"))
				req.Header.Set("Content-Type", "application/json")
				begin := time.Now()
				resp, err := client.Do(req)
				if err == nil {
					var result struct {
						Allowed bool  `json:"allowed"`
						Limit   int64 `json:"limit"`
					}
					err = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&result)
					resp.Body.Close()
					if err == nil && result.Limit > 0 && ((resp.StatusCode == 200 && result.Allowed) || (resp.StatusCode == 429 && !result.Allowed)) {
						outcomes[i] = resp.StatusCode
					} else {
						outcomes[i] = -1
					}
				} else {
					outcomes[i] = -1
				}
				samples[i] = float64(time.Since(begin).Nanoseconds()) / 1e6
			}
		}()
	}
	for i := 0; i < *n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start).Seconds()
	allowed, denied, failed := 0, 0, 0
	sum := 0.0
	for i, v := range outcomes {
		sum += samples[i]
		switch v {
		case 200:
			allowed++
		case 429:
			denied++
		default:
			failed++
		}
	}
	report := map[string]any{"timestamp": time.Now().UTC(), "go_version": runtime.Version(), "cpu_count": runtime.NumCPU(), "mode": *mode, "requests": *n, "concurrency": *workers, "allowed": allowed, "denied": denied, "errors": failed, "duration_seconds": elapsed, "requests_per_second": float64(*n) / elapsed, "mean_ms": sum / float64(*n), "p50_ms": loadtest.Percentile(samples, .5), "p95_ms": loadtest.Percentile(samples, .95), "p99_ms": loadtest.Percentile(samples, .99), "latency_scope": "HTTP round trip including body decode; all outcomes; no warmup"}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
	if failed > 0 {
		os.Exit(1)
	}
}
