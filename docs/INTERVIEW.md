# Engineering explanation

## What it does

Trusted backend services ask whether an identified client's operation fits its
resource quota. HTTP and gRPC return the same admission decision. Redis stores the
shared usage so adding application replicas does not multiply available quota.

## Why this architecture

Preserve the project's two interfaces and Go/Redis stack. Separate policy from
storage behind a small Go interface so validation and failure handling can be
unit-tested without Redis. Use real Redis for concurrency and expiry guarantees.
A Lua script combines prune, count, decision, insertion, and TTL into one serialized
operation. A pipeline alone does not provide that decision boundary.

## Hard problems addressed

The old member format reused timestamps and costs, so requests could overwrite
one another. A random request prefix plus a per-unit suffix removes that collision
mechanism. Redis server time avoids skew between application clocks. Checking the
budget before insertion prevents denied calls from exhausting or extending quotas.
Weighted requests are all-or-nothing and capped to keep Lua work bounded.

Exactness costs memory: each admitted unit occupies a sorted-set member. This
makes the implementation explainable and testable but a poor fit for huge quotas.
A GCRA/token-bucket variant would trade different semantics for bounded key size.

## Failure scenarios and consistency

All decisions on one Redis primary are serialized. Requests may time out after
execution; retries are not idempotent and can consume again. Redis outages return
503/Unavailable and callers must not interpret them as allowance. Redis eviction
would reset budgets, so Compose uses noeviction. AOF reduces restart loss but does
not prove durability of every acknowledged decision; failover and Redis Cluster
are not implemented. Quota resets are privileged and deliberately restore budget.

## Security

Service API keys are for trusted infrastructure, not end users. The gateway must
authenticate users and map them to quota identifiers; otherwise a caller could
switch identifiers. Admin credentials are distinct. Inputs and request sizes are
bounded. Secrets stay out of code and logs. Deployment requires private networking
and TLS termination because the service's listeners are plaintext.

## Performance evidence

Use docs/VALIDATION.md and the raw reports. Explain the hardware/runtime, sample
count, concurrency, closed-loop workload, absence of warmup, and whether traffic
was admitted or denied. Shared-key throughput includes fast rejections; it is not
a sustained all-admitted throughput claim. p99 is computed from sorted samples,
not the 99th-position unsorted response. No validated baseline comparison exists.

## Next scale steps

Measure sustained representative traffic, Redis memory per active identity, and
tail latency under noisy neighbors. Evaluate a lower-memory algorithm if necessary.
Add coordinated versioned policy rollout, high-availability Redis with explicit
failover consistency expectations, TLS/mTLS configuration, and idempotency keys
only if callers need retry-safe admission. Each adds operational complexity that
must be justified and tested.
