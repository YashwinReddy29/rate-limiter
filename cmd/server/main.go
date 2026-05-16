package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"google.golang.org/grpc"

	pb "github.com/YashwinReddy29/rate-limiter/proto"
	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
	"github.com/YashwinReddy29/rate-limiter/internal/store"
)

func main() {
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	grpcPort  := getEnv("GRPC_PORT", "50051")
	httpPort  := getEnv("HTTP_PORT", "8080")

	rs := store.NewRedisStore(redisAddr)
	if err := rs.Ping(context.Background()); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	log.Printf("✓ Redis connected at %s", redisAddr)

	rl := limiter.New(rs)

	// gRPC
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("gRPC listen failed: %v", err)
	}
	grpcSrv := grpc.NewServer()
	pb.RegisterRateLimiterServer(grpcSrv, &grpcServer{rl: rl})

	go func() {
		log.Printf("✓ gRPC listening on :%s", grpcPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// HTTP
	httpSrv := &httpServer{rl: rl}
	log.Printf("✓ HTTP listening on :%s", httpPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", httpPort), httpSrv.routes()); err != nil {
		log.Fatalf("HTTP serve error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}