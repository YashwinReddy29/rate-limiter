package main

import (
	"context"
	"fmt"
	pb "github.com/YashwinReddy29/rate-limiter/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"log"
	"os"
	"time"
)

func main() {
	// Plaintext is for loopback development only. Use TLS at the deployment boundary.
	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50051"
	}
	if os.Getenv("API_KEY") == "" {
		log.Fatal("API_KEY required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", os.Getenv("API_KEY"))
	resp, err := pb.NewRateLimiterClient(conn).Check(ctx, &pb.CheckRequest{ClientId: "example", Resource: "api", Cost: 1})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("allowed=%v remaining=%d limit=%d reset_after_ms=%d\n", resp.Allowed, resp.Remaining, resp.Limit, resp.ResetAfterMs)
}
