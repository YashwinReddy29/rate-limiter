package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
	"github.com/YashwinReddy29/rate-limiter/internal/store"
	pb "github.com/YashwinReddy29/rate-limiter/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + getEnv("HTTP_PORT", "8080") + "/ready")
		if err != nil {
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("server_stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	auth := credentials{service: os.Getenv("API_KEY"), admin: os.Getenv("ADMIN_API_KEY")}
	if err := auth.validate(); err != nil {
		return err
	}
	rs := store.NewWithOptions(&redis.Options{Addr: getEnv("REDIS_ADDR", "127.0.0.1:6379"), Username: os.Getenv("REDIS_USERNAME"), Password: os.Getenv("REDIS_PASSWORD")})
	defer rs.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := rs.Ping(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("Redis startup check: %w", err)
	}
	rl := limiter.New(rs)
	m := &metrics{}
	lis, err := net.Listen("tcp", net.JoinHostPort(getEnv("BIND_ADDR", "127.0.0.1"), getEnv("GRPC_PORT", "50051")))
	if err != nil {
		return err
	}
	defer lis.Close()
	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(auth.intercept), grpc.MaxRecvMsgSize(4096), grpc.MaxConcurrentStreams(128))
	pb.RegisterRateLimiterServer(grpcSrv, &grpcServer{rl: rl, metrics: m})
	httpSrv := &http.Server{Addr: net.JoinHostPort(getEnv("BIND_ADDR", "127.0.0.1"), getEnv("HTTP_PORT", "8080")), Handler: (&httpServer{rl: rl, auth: auth, metrics: m}).routes(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	stopped, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- grpcSrv.Serve(lis) }()
	go func() { errs <- httpSrv.ListenAndServe() }()
	slog.Info("server_started", "http", httpSrv.Addr, "grpc", lis.Addr().String())
	select {
	case <-stopped.Done():
	case err = <-errs:
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	grpcDone := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(grpcDone) }()
	_ = httpSrv.Shutdown(shutdownCtx)
	select {
	case <-grpcDone:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
	}
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
