# Validate the current main branch in WSL2

Production hardening is merged into `main`. Use this guide to reproduce the Docker,
test, security-scan, and benchmark checks locally. Historical validation evidence is
recorded in `docs/VALIDATION.md`.

Run in Ubuntu WSL (use a fresh directory):

```bash
cd ~
git clone https://github.com/YashwinReddy29/rate-limiter.git
cd rate-limiter
export API_KEY=$(openssl rand -hex 32)
export ADMIN_API_KEY=$(openssl rand -hex 32)
docker compose config --quiet
docker compose up --build -d --wait --wait-timeout 120
curl --fail http://127.0.0.1:8080/ready
```

Keep that terminal open to retain the temporary keys. Once startup succeeds:

```bash
go mod download
REDIS_TEST_ADDR=127.0.0.1:6379 go test -race -count=1 ./...
go vet ./...
go build ./...
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
"$(go env GOPATH)/bin/govulncheck" ./...
curl --fail -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' \
  -d '{"client_id":"wsl","resource":"api","cost":1}' \
  http://127.0.0.1:8080/check
go run ./cmd/client
go run ./cmd/benchmark -n 10000 -concurrency 50 -mode shared
go run ./cmd/benchmark -n 10000 -concurrency 50 -mode unique
git diff --check
git status --short
```

The benchmark commands above print your WSL results without overwriting the
checked-in preparation-environment reports. Preserve new measurements with their
machine/workload details if using them in resume claims.

After the checks pass, keep any benchmark output you plan to cite together with the
machine/runtime details. Compare your local commit with `origin/main` before using the
results as evidence:

```bash
git fetch origin
git status --short --branch
git rev-parse HEAD
git rev-parse origin/main
```
