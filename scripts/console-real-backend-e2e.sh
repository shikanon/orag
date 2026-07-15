#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

compose=(docker compose -p orag-console-e2e -f deployments/docker-compose.test.yml)
database_url="postgres://orag:orag@127.0.0.1:55432/orag_console_e2e?sslmode=disable"
api_log="$root/.tmp/console-real-backend-e2e/api.log"
api_pid=""

if [[ -n "${ORAG_NODE_BIN:-}" ]]; then
  export PATH="$(dirname "$ORAG_NODE_BIN"):$PATH"
fi
export CGO_ENABLED=0
export GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}"
export STORAGE_BACKEND=qdrant_postgres
export DATABASE_URL="$database_url"
export QDRANT_HOST=127.0.0.1
export QDRANT_GRPC_PORT=6634
export QDRANT_COLLECTION=orag_console_e2e_chunks
export QDRANT_SEMANTIC_CACHE_COLLECTION=orag_console_e2e_semantic_cache
export QDRANT_AUTO_CREATE_COLLECTIONS=true
export JWT_SECRET=orag-console-e2e-jwt-secret
export API_KEY_PEPPER=orag-console-e2e-api-key-pepper
export ADMIN_DEFAULT_USERNAME=e2e-admin
export ADMIN_DEFAULT_PASSWORD=e2e-password
export ALLOW_DETERMINISTIC_MOCK=true
export LLM_CHAT_PROVIDER=mock
export LLM_EMBEDDING_PROVIDER=mock
export LLM_RERANK_PROVIDER=mock
export LLM_MULTIMODAL_PROVIDER=mock
export PORT=18080
export ORAG_CONSOLE_API_TARGET=http://127.0.0.1:18080
export ORAG_REAL_BACKEND_E2E=1

cleanup() {
  local status=$?
  if [[ -n "$api_pid" ]] && kill -0 "$api_pid" 2>/dev/null; then
    kill "$api_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true
  fi
  "${compose[@]}" logs --no-color > "$root/.tmp/console-real-backend-e2e/dependencies.log" 2>&1 || true
  "${compose[@]}" down -v >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

mkdir -p "$root/.tmp/console-real-backend-e2e"
"${compose[@]}" up -d --wait
if ! "${compose[@]}" exec -T postgres psql -U orag -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'orag_console_e2e'" | grep -qx '1'; then
  "${compose[@]}" exec -T postgres createdb -U orag orag_console_e2e
fi
go run ./cmd/oragctl migrate
go run ./cmd/orag-api >"$api_log" 2>&1 &
api_pid=$!
WAIT_READY_ATTEMPTS=120 ./scripts/wait-ready.sh http://127.0.0.1:18080/readyz
npm --prefix console run test:e2e -- e2e/real-backend-release.spec.ts
