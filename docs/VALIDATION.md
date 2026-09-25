# Validation

Baseline: `ad37b60`. Production hardening and the follow-up gRPC security update are
merged into `main`. Pull request #2 passed the full CI workflow before merge, including
race-enabled tests, `govulncheck`, Docker Compose startup, readiness, and an authenticated
HTTP smoke test. Verify the latest `main` GitHub Actions run for the current commit before
making a release claim.

## Executed checks

- Go 1.26.7 on Linux amd64; real Redis 7.4.2 compiled from its release tag.
- `go test -race -count=1 -json ./...`: 11 top-level tests and 17 subtests passed;
  no test cases skipped. REDIS_TEST_ADDR pointed to a supervised local Redis.
- `go vet ./...`, `go build ./...`, `go mod verify`, formatting, and
  `git diff --check`: passed.
- `govulncheck` is enforced in CI. The final merged dependency set includes gRPC
  `v1.83.2`, which resolved GO-2026-6443 and GO-2026-6348 identified during hardening.
- HTTP readiness and authenticated gRPC client: passed.
- SIGTERM shutdown: process exited successfully with status 0.
- `.env`, `.env.production`, and build output are ignored. No secret files staged.

The initial baseline `go test` failed a client vet check and had no automated tests.
One intermediate run failed because the temporary Redis process had exited; those
failures were diagnosed and the complete validation repeated with supervised Redis.
The reports contain the final successful run.

## Measured HTTP workloads

Both runs use 10000 requests, 50 closed-loop workers, no warmup, the service,
load generator, and Redis on the same shared host (9 exposed logical CPUs).
Each workload finishes in roughly one second; these are smoke benchmarks, not
sustained capacity, soak, production, or WSL results. Logging was enabled.

| Workload | Allowed | Denied | Errors | Requests/s | Mean ms | p95 ms | p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Shared client / 1000-unit quota | 1000 | 9000 | 0 | 11590.55 | 3.91 | 12.03 | 23.51 |
| Unique client per request | 10000 | 0 | 0 | 10546.90 | 4.37 | 13.19 | 19.13 |

The concurrency correctness test separately issues 200 calls through two independent
Redis clients and asserts exactly 50 admissions for a 50-unit quota. Weighted-cost,
denied-usage, expiry, and reset behavior are asserted, not inferred from throughput.

Raw measurements: reports/benchmark-shared.json, reports/benchmark-unique.json.
Test events: reports/tests.jsonl. Audit output: reports/security-audit.txt.
No before/after performance percentage is valid because the old benchmark was not
correct. Do not reuse the removed 0.215 ms p99 or ~50k requests/sec README claims.

## Release guidance

GitHub Actions validates the application on pull requests and on pushes to `main`.
For machine-specific resume or interview performance claims, rerun the benchmark in
WSL or another named environment and preserve the raw output together with CPU count,
request count, concurrency, and workload mode. Do not reuse the removed baseline
`0.215 ms p99` / `~50k requests/sec` claims.
