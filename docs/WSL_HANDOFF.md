# Finish validation in WSL2

The production-upgrade branch is published for local Docker validation. Source
tests, dependency audits, and benchmarks are recorded in docs/VALIDATION.md.
Verify GitHub Actions and your WSL checks before merging into main.

Run in Ubuntu WSL (use a fresh directory):

```bash
cd ~
git clone -b production-upgrade https://github.com/YashwinReddy29/rate-limiter.git rate-limiter-upgrade
cd rate-limiter-upgrade
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

Only after the results pass and are reviewed:

```bash
git fetch origin
git log --oneline --left-right origin/main...production-upgrade
git push -u origin production-upgrade
```

Verify Actions on that exact commit and resolve any failures before merging.
Do not merge main automatically. An upgrade PR in your own repository and a
contribution PR to another maintainer's repository are separate activities.
