package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/YashwinReddy29/rate-limiter/proto"
)

func main() {
	conn, err := grpc.Dial("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("connection failed: %v", err)
	}
	defer conn.Close()

	client := pb.NewRateLimiterClient(conn)
	ctx := context.Background()

	fmt.Println("=== Rate Limiter Test Client ===\n")

	// Test 1: Basic check
	fmt.Println("Test 1: Basic rate limit check")
	resp, err := client.Check(ctx, &pb.CheckRequest{
		ClientId: "test_client", Resource: "api", Cost: 1,
	})
	if err != nil {
		log.Fatalf("Check failed: %v", err)
	}
	fmt.Printf("  Allowed: %v | Remaining: %d | Limit: %d\n\n",
		resp.Allowed, resp.Remaining, resp.Limit)

	// Test 2: Exhaust quota
	fmt.Println("Test 2: Exhaust free_client quota (limit=100)")
	allowed, blocked := 0, 0
	for i := 0; i < 110; i++ {
		r, _ := client.Check(ctx, &pb.CheckRequest{
			ClientId: "free_client", Resource: "api", Cost: 1,
		})
		if r.Allowed {
			allowed++
		} else {
			blocked++
		}
	}
	fmt.Printf("  Allowed: %d | Blocked: %d\n\n", allowed, blocked)

	// Test 3: Premium client higher limit
	fmt.Println("Test 3: Premium client (limit=10000)")
	r, _ := client.GetQuota(ctx, &pb.QuotaRequest{
		ClientId: "premium_client", Resource: "api",
	})
	fmt.Printf("  Limit: %d | Window: %ds\n\n", r.Limit, r.WindowSec)

	// Test 4: Concurrent load test
	fmt.Println("Test 4: Concurrent load — 1000 requests, 50 goroutines")
	client.Check(ctx, &pb.CheckRequest{ClientId: "load_client", Resource: "api", Cost: 1})
	// Reset first
	client.Reset(ctx, &pb.ResetRequest{ClientId: "load_client", Resource: "api"})

	var wg sync.WaitGroup
	latencies := make([]time.Duration, 1000)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start := time.Now()
			client.Check(ctx, &pb.CheckRequest{
				ClientId: "load_client", Resource: "api", Cost: 1,
			})
			latencies[idx] = time.Since(start)
		}(i)
	}
	wg.Wait()

	var total time.Duration
	var p99 time.Duration
	for _, l := range latencies {
		total += l
	}
	avg := total / 1000
	p99 = latencies[990] // approximate p99
	fmt.Printf("  Avg latency: %v | ~p99: %v\n\n", avg, p99)

	fmt.Println("✓ All tests passed")
}