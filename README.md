# Distributed Rate Limiter as a Service

A Go service that lets trusted backend applications share per-client, per-resource
quotas through HTTP and gRPC. Redis executes each sliding-window admission decision
atomically. The service is intended for private infrastructure behind a TLS ingress.

## Architecture

HTTP and gRPC adapters authenticate callers, bound requests, and invoke the same
limiter policy. A single Redis Lua script prunes expired units, checks the remaining
budget, and records admitted units. Redis TIME supplies the clock. Each accepted
unit has a cryptographically random request prefix and a unit index.

This is an **exact sliding-window log**, not an approximate sliding-window counter.
The default window is 60 seconds. Usage includes accepted cost units in
`(now - window, now]`. Rejected calls do not consume quota or refresh expiration.
A zero-cost internal operation reads usage without consuming it.

The checked-in quotas in `internal/limiter/config.go` preserve the original tiers:

| Resource | Default units / 60s |
| --- | ---: |
| api | 1000 |
| search | 50 |
| upload | 10 |
| auth | 5 |
| other resources | 100 |

`free_client` overrides api/search to 100/10; `premium_client` to 10000/500.
Policy changes require a coordinated rebuild/redeploy. All replicas must run the
same policy. There is no dynamic policy API or policy database.

## Setup in Ubuntu / WSL2

Requirements: Go 1.26.7 or a newer patched compatible toolchain, Docker Engine with
Compose v2 (or Docker Desktop with WSL integration), Git, and OpenSSL.

```bash
git clone https://github.com/YashwinReddy29/rate-limiter.git
cd rate-limiter
git switch production-upgrade
cp .env.example .env
```

Generate two DIFFERENT secrets and set API_KEY and ADMIN_API_KEY in `.env`:

```bash
openssl rand -hex 32
openssl rand -hex 32
```

The server requires distinct keys of at least 32 characters. The Go executable does
not automatically load `.env`; load your locally generated file before local runs:

```bash
set -a
source .env
set +a
go mod download
docker compose up -d redis
go run ./cmd/server
```

In another shell, load `.env` again for commands requiring credentials. Compose
reads `.env` automatically:

```bash
docker compose config --quiet
docker compose up --build -d --wait
docker compose ps
curl --fail http://127.0.0.1:8080/ready
```

Local Go and the Compose application use the same ports; run one application at a
time. Stop the local Go process before starting the complete Compose stack.

## Environment

| Variable | Meaning / default |
| --- | --- |
| API_KEY | Required trusted-service credential |
| ADMIN_API_KEY | Required separate reset/metrics credential |
| REDIS_ADDR | `127.0.0.1:6379` locally; `redis:6379` in Compose |
| REDIS_USERNAME / REDIS_PASSWORD | Optional Redis ACL credentials |
| BIND_ADDR | `127.0.0.1` locally; `0.0.0.0` inside Compose |
| HTTP_PORT / GRPC_PORT | `8080` / `50051` |
| REDIS_TEST_ADDR | Explicit Redis address for integration tests |

`.env` and `.env.*` are ignored except `.env.example`. Do not commit credentials.

## HTTP contract

Send `X-API-Key` on protected endpoints. Responses use snake_case JSON.

| Method | Path | Credential | Behavior |
| --- | --- | --- | --- |
| POST | /check | Service or admin | 200 admitted, 429 denied |
| GET | /quota?client_id=...&resource=... | Service or admin | Current usage without consuming |
| POST | /reset | Admin | Clear one client/resource quota |
| GET | /health | None | Process liveness |
| GET | /ready | None | 200 if Redis responds, otherwise 503 |
| GET | /metrics | Admin | Prometheus text exposition |

```bash
curl -i -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' \
  -d '{"client_id":"example","resource":"api","cost":1}' \
  http://127.0.0.1:8080/check

curl --fail -H "X-API-Key: $API_KEY" \
  'http://127.0.0.1:8080/quota?client_id=example&resource=api'

curl --fail -H "X-API-Key: $ADMIN_API_KEY" -H 'Content-Type: application/json' \
  -d '{"client_id":"example","resource":"api"}' \
  http://127.0.0.1:8080/reset
```

Identifiers must match `[A-Za-z0-9_.-]{1,128}`. HTTP cost defaults to 1 when omitted;
explicit zero, null, negative, or excessive costs are invalid. Costs range from 1
to 10000. A cost above the selected quota is denied without allocating Redis state.
Requests are limited to 4 KiB, unknown JSON fields and trailing objects are rejected.
Validation errors return 400, missing/invalid authentication 401, unauthorized
administration 403, and backend failures 503 without internal connection details.

`reset_after_ms` is time until all currently accepted units expire if no more are
accepted. HTTP `Retry-After` rounds this duration UP to seconds on exhausted quotas;
it is deliberately conservative, not the earliest possible retry for a small cost.
Other callers may consume capacity while a caller waits. An impossible cost has no
retry delay. `X-RateLimit-Limit` and `X-RateLimit-Remaining` report the decision.

## gRPC

`proto/ratelimiter.proto` remains the wire contract: Check, Reset, GetQuota. Pass
`x-api-key` metadata. Unlike HTTP omission, proto3 scalar cost 0 is invalid; set 1.
A quota denial is a normal CheckResponse with `allowed=false`. Validation, auth,
authorization, and backend errors map to InvalidArgument, Unauthenticated,
PermissionDenied, and Unavailable respectively.

```bash
go run ./cmd/client
```

Both protocols generate request IDs. HTTP exposes X-Request-ID; gRPC returns
x-request-id metadata. Neither logs supplied API keys or request bodies.

## Reliability and security boundaries

- Admission is atomic **on one Redis primary**, including across application replicas.
- Redis client retries are disabled for ambiguous mutation failures. A timeout can
  occur after Redis consumed quota; blindly retrying can charge again. This is not
  an exactly-once or idempotent API.
- A two-second operation deadline and bounded Redis connection/pool timeouts prevent
  indefinite waits. HTTP has header/read/write/idle timeouts. SIGINT/SIGTERM drains
  both servers with a five-second shutdown budget.
- Redis failure produces unavailable responses, not permission to proceed. Upstream
  callers must treat unavailable as a failed decision and must not fail open silently.
- Compose uses noeviction: silently evicting a live quota key would restore capacity.
  When memory fills, admissions can fail; alert and capacity-plan accordingly.
- Compose enables AOF and a named volume. Default AOF durability can lose recent
  decisions after a crash. This is not a highly available Redis topology.
- State uses an `rl:v2:` namespace. Upgrade from the old implementation starts fresh
  quotas. Do not mix old and new replicas during cutover.
- Credentials authorize **trusted services to select client IDs**. These are not
  end-user tenant credentials. Authenticate end users upstream and derive their
  identifiers there. Never expose this API directly to untrusted browsers.
- Compose publishes loopback ports only; its Redis development service has no password.
  For remote deployment use private networking, Redis ACLs, and a TLS termination
  proxy supporting HTTP and gRPC. Native TLS, mTLS, and automated key rotation are
  not implemented here.
- The application container runs non-root with a read-only filesystem and dropped
  capabilities. It contains its own readiness healthcheck command.

## Observability

Structured JSON request logs include generated correlation IDs and durations.
Admin-only `/metrics` exposes HTTP request count, allowed/denied/error decision
counts across both protocols, and decision duration sum/count in seconds. These
metrics reset when the process restarts; no high-cardinality client labels are used.
The duration metric measures limiter execution, not end-to-end HTTP latency, and
it does not publish p99. Use the standalone benchmark for latency distributions.

## Tests and validation

```bash
go mod verify
go vet ./...
REDIS_TEST_ADDR=127.0.0.1:6379 go test -race -count=1 ./...
go build ./...
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
govulncheck ./...
git diff --check
```

Without REDIS_TEST_ADDR, real Redis integration tests explicitly skip. A release
validation must set it. Tests cover two-client concurrent admission, weighted costs,
denial accounting, expiry, reset, namespace isolation, strict HTTP parsing, role
separation, HTTP/gRPC error mapping, and percentile calculation. CI also builds and
starts Compose, checks readiness, and sends an authenticated HTTP request.

The original baseline failed `go test ./...` due to a vet error in the demo client;
it had no automated tests. Its claimed 0.215 ms p99 and ~50k requests/second are
removed: the original percentile code indexed unsorted observations and did not
validate outcomes. No before/after performance improvement is claimed.

## Reproducible benchmark

Start the service first, then:

```bash
mkdir -p reports
go run ./cmd/benchmark -n 10000 -concurrency 50 -mode shared > reports/benchmark-shared.json
go run ./cmd/benchmark -n 10000 -concurrency 50 -mode unique > reports/benchmark-unique.json
```

Each run creates a fresh client prefix. `shared` exercises one 1000-unit quota,
including denied responses. `unique` allocates a separate client per request,
exercising admitted traffic and key creation. Neither result is interchangeable
with an admitted-only sustained hot-key capacity figure. The workers run a closed
loop with no warmup; percentiles use sorted nearest-rank samples, including all
outcomes, and timing covers HTTP request through decoded response body. Errors
cause a nonzero exit. The JSON records counts, concurrency, runtime, CPU count,
elapsed time, throughput, mean, p50, p95, and p99.

Measured results and validation limitations are in `docs/VALIDATION.md`, with raw
reports in `reports/`. Rerun on WSL for machine-specific resume/interview evidence.

## Tradeoffs and further work

Exact sliding-window logs use O(accepted units) memory per active key and bounded
O(cost log N) admission work. Cost and policies cap each key at 10000 units, but
total active identities still require capacity planning and trusted callers.
Long windows or very high quotas may justify a token-bucket/GCRA design. Redis is
one failure domain; failover, durable idempotency, dynamic policy versioning, TLS
configuration, richer histograms, and sustained soak tests are future work, not
implemented features.

See `docs/UPSTREAM_CONTRIBUTION.md` for a checked open-source issue candidate and
`docs/INTERVIEW.md` for design explanations. No external contribution is claimed.
