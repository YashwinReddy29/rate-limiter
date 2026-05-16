# Distributed Rate Limiter as a Service

Multi-tenant rate limiting microservice with sliding window counters, per-client quota configuration, and gRPC + HTTP interfaces.

## Stack
Go 1.22 · Redis 7 · gRPC · Protocol Buffers · Docker

## Performance
| Metric | Value |
|--------|-------|
| p99 latency | 0.215ms |
| avg latency | 0.20ms |
| target | <4ms p99 |
| throughput | ~50k req/sec |

## Features
- Sliding window counter via Redis sorted sets
- Per-client quota overrides (premium vs free tiers)
- gRPC interface for upstream service consumption
- HTTP interface for admin/debug
- Pipeline Redis commands for atomic operations
- Benchmark endpoint measuring real p99

## Run locally
```bash
docker compose up -d redis
go run ./cmd/server

# Test
go run ./cmd/client
curl -X POST http://localhost:8080/check \
  -d '{"client_id":"my_client","resource":"api","cost":1}'
```

## API
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/check` | POST | Check and consume quota |
| `/quota` | GET | View current usage |
| `/reset` | GET | Reset client quota |
| `/benchmark` | GET | Measure p99 latency |
