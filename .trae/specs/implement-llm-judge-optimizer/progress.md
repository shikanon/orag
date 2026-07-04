# Progress

## 2026-07-04 Task 12
- Implemented async optimizer service with run/candidate state machine, persisted checkpoints, cooperative cancel, resume, budget hard stops, rate limiter hook, cost counters, and holdout re-evaluation.
- Added optimizer repository models and PostgreSQL methods for `optimization_runs`, `optimization_candidates`, and `harness_runs`.
- Added service tests for async submit/polling, cancel, resume after budget stop, checkpoint skip, runner failure, and holdout isolation.
- Added PostgreSQL repository tests for optimizer run/candidate/harness JSONB encoding and tenant-guarded candidate listing.
- Verified with `CGO_ENABLED=0 GOFLAGS='-tags=stdjson,gjson' go test ./internal/optimizer ./internal/storage/postgres -v`.
- `GOTOOLCHAIN=local` could not run in this environment because the local Go is `1.22.5` while `go.mod` requires `go >= 1.26`; default toolchain test passed.

## 2026-07-04 Task 13
- Wired the app to the async optimizer service, including a memory optimizer repository for local/test runs and the existing PostgreSQL repository for persistent runs.
- Replaced `POST /v1/optimizations` with async submission returning `run_id`, `poll_url`, `cancel_url`, and `resume_url`; added `GET /v1/optimizations/{id}`, `POST /v1/optimizations/{id}:cancel`, and `POST /v1/optimizations/{id}:resume`.
- Preserved legacy `profiles/top_ks` optimize request compatibility by mapping it into internal RAG profile metadata and retrieval top-k search space.
- Exposed Judge/QAG request schema and evaluation detail schema in OpenAPI; added optimization accepted/status/run/candidate schemas and contract tests.
- Updated `docs/api.md`, `docs/evaluation.md`, `examples/curl/40_eval.sh`, `examples/curl/lib.sh`, and added `examples/curl/50_optimize.sh` for async optimization smoke usage.
- Verified with `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http ./internal/optimizer ./tests/contract -v`.

## 2026-07-04 Task 14
- Ran the required Task 14 verification commands with `GOTOOLCHAIN=local`; all three were blocked by the local Go version because this machine has `go1.22.5` while `go.mod` requires `go >= 1.26`.
- Re-ran the same package sets without `GOTOOLCHAIN=local`, allowing Go's default toolchain selection, and verified `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/eval ./internal/optimizer -v`.
- Verified storage, HTTP and OpenAPI contract coverage with `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres ./internal/http ./tests/contract -v`.
- Verified the full repository with `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./...`.
- Updated Task 14 and the final checklist to reflect the completed validation and documented environment limitation.
