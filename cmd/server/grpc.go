package main

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
	pb "github.com/YashwinReddy29/rate-limiter/proto"
)

type grpcServer struct {
	pb.UnimplementedRateLimiterServer
	rl      *limiter.RateLimiter
	metrics *metrics
}

func (s *grpcServer) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	started := time.Now()
	result, err := s.rl.Check(ctx, req.ClientId, req.Resource, req.Cost)
	s.metrics.observe(result, err, time.Since(started))
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.CheckResponse{
		Allowed:      result.Allowed,
		Remaining:    result.Remaining,
		Limit:        result.Limit,
		ResetAfterMs: result.ResetAfterMs,
		Reason:       result.Reason,
	}, nil
}

func (s *grpcServer) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	err := s.rl.Reset(ctx, req.ClientId, req.Resource)
	return &pb.ResetResponse{Success: err == nil}, rpcError(err)
}

func (s *grpcServer) GetQuota(ctx context.Context, req *pb.QuotaRequest) (*pb.QuotaResponse, error) {
	info, err := s.rl.GetQuota(ctx, req.ClientId, req.Resource)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.QuotaResponse{
		ClientId:  info.ClientID,
		Resource:  info.Resource,
		Limit:     info.Limit,
		Used:      info.Used,
		Remaining: info.Remaining,
		WindowSec: info.WindowSec,
	}, nil
}
func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, limiter.ErrInvalid) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	return status.Error(codes.Unavailable, "rate limiter unavailable")
}
