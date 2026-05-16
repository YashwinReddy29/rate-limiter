package main

import (
	"context"

	pb "github.com/YashwinReddy29/rate-limiter/proto"
	"github.com/YashwinReddy29/rate-limiter/internal/limiter"
)

type grpcServer struct {
	pb.UnimplementedRateLimiterServer
	rl *limiter.RateLimiter
}

func (s *grpcServer) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	result, err := s.rl.Check(ctx, req.ClientId, req.Resource, req.Cost)
	if err != nil {
		return nil, err
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
	return &pb.ResetResponse{Success: err == nil}, err
}

func (s *grpcServer) GetQuota(ctx context.Context, req *pb.QuotaRequest) (*pb.QuotaResponse, error) {
	info, err := s.rl.GetQuota(ctx, req.ClientId, req.Resource)
	if err != nil {
		return nil, err
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