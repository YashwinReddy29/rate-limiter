package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type credentials struct{ service, admin string }

func (c credentials) validate() error {
	if len(c.service) < 32 || len(c.admin) < 32 || c.service == c.admin {
		return errors.New("API_KEY and ADMIN_API_KEY must be distinct and at least 32 characters")
	}
	return nil
}
func equal(a, b string) bool {
	x := sha256.Sum256([]byte(a))
	y := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}
func (c credentials) authorized(key string, admin bool) bool {
	if key == "" {
		return false
	}
	if equal(key, c.admin) {
		return true
	}
	return !admin && equal(key, c.service)
}
func (c credentials) intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	started := time.Now()
	var id [16]byte
	_, _ = rand.Read(id[:])
	requestID := hex.EncodeToString(id[:])
	_ = grpc.SetHeader(ctx, metadata.Pairs("x-request-id", requestID))
	defer func() {
		slog.Info("grpc_request", "request_id", requestID, "method", info.FullMethod, "duration_ms", time.Since(started).Milliseconds())
	}()
	md, _ := metadata.FromIncomingContext(ctx)
	keys := md.Get("x-api-key")
	if len(keys) != 1 || !c.authorized(keys[0], false) {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if info.FullMethod == "/ratelimiter.RateLimiter/Reset" && !c.authorized(keys[0], true) {
		return nil, status.Error(codes.PermissionDenied, "administrator required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return handler(ctx, req)
}
