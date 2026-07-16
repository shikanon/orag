#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

compose=(docker compose -p orag-tutorial-clone-e2e -f deployments/docker-compose.test.yml)
database_url="postgres://orag:orag@127.0.0.1:55432/orag_tutorial_clone_e2e?sslmode=disable"
tmp="$root/.tmp/console-real-tutorial-clone-e2e"
private_output="$tmp/private-output"
public_catalog="$tmp/public-catalog"
api_log="$tmp/api.log"
fixture_log="$tmp/fixture.log"
api_pid=""
fixture_pid=""

if [[ -n "${ORAG_NODE_BIN:-}" ]]; then
  export PATH="$(dirname "$ORAG_NODE_BIN"):$PATH"
fi
export CGO_ENABLED=0
export GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}"
export STORAGE_BACKEND=qdrant_postgres
export DATABASE_URL="$database_url"
export QDRANT_HOST=127.0.0.1
export QDRANT_GRPC_PORT=6634
export QDRANT_COLLECTION=orag_tutorial_clone_e2e_chunks
export QDRANT_SEMANTIC_CACHE_COLLECTION=orag_tutorial_clone_e2e_semantic_cache
export QDRANT_AUTO_CREATE_COLLECTIONS=true
export JWT_SECRET=orag-tutorial-clone-e2e-jwt-secret
export API_KEY_PEPPER=orag-tutorial-clone-e2e-api-key-pepper
export ADMIN_DEFAULT_USERNAME=e2e-admin
export ADMIN_DEFAULT_PASSWORD=e2e-password
export ALLOW_DETERMINISTIC_MOCK=true
export LLM_CHAT_PROVIDER=mock
export LLM_EMBEDDING_PROVIDER=mock
export LLM_RERANK_PROVIDER=mock
export LLM_MULTIMODAL_PROVIDER=mock
export PORT=18081
export ORAG_CONSOLE_API_TARGET=http://127.0.0.1:18081
export ORAG_REAL_TUTORIAL_CLONE_E2E=1
export ORAG_TEST_MODE=true
export TUTORIAL_CATALOG_BASE_URL=http://127.0.0.1:18082
export TUTORIAL_PRIVATE_OUTPUT_DIR="$private_output"
export OBJECT_STORAGE_PROVIDER=local

cleanup() {
  local status=$?
  if [[ -n "$api_pid" ]] && kill -0 "$api_pid" 2>/dev/null; then
    kill "$api_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true
  fi
  if [[ -n "$fixture_pid" ]] && kill -0 "$fixture_pid" 2>/dev/null; then
    kill "$fixture_pid" 2>/dev/null || true
    wait "$fixture_pid" 2>/dev/null || true
  fi
  "${compose[@]}" logs --no-color > "$tmp/dependencies.log" 2>&1 || true
  "${compose[@]}" down -v >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

mkdir -p "$tmp" "$private_output"
# The embedded production catalog intentionally remains on the already-published
# 1.0.0 Pack. For this controlled browser test, serve the immutable 1.0.1 JSON
# candidate fixture through the 1.0.0 catalog path after rewriting only the
# temporary manifest version. No source fixture or public Pack is modified.
cp -R "$root/tests/fixtures/tutorial-packs" "$public_catalog"
rm -rf "$public_catalog/text-rag/1.0.0/quick"
cp -R "$root/tests/fixtures/tutorial-packs/text-rag/1.0.1/quick" "$public_catalog/text-rag/1.0.0/quick"
sed -i.bak 's/"version": "1.0.1"/"version": "1.0.0"/' "$public_catalog/text-rag/1.0.0/quick/manifest.json"
rm "$public_catalog/text-rag/1.0.0/quick/manifest.json.bak"
"${compose[@]}" up -d --wait
if ! "${compose[@]}" exec -T postgres psql -U orag -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'orag_tutorial_clone_e2e'" | grep -qx '1'; then
  "${compose[@]}" exec -T postgres createdb -U orag orag_tutorial_clone_e2e
fi

python3 -m http.server 18082 --bind 127.0.0.1 --directory "$public_catalog" >"$fixture_log" 2>&1 &
fixture_pid=$!
WAIT_READY_ATTEMPTS=60 ./scripts/wait-ready.sh http://127.0.0.1:18082/text-rag/1.0.0/quick/manifest.json

go run ./cmd/oragctl migrate
go build -o "$tmp/orag-api" ./cmd/orag-api
"$tmp/orag-api" >"$api_log" 2>&1 &
api_pid=$!
WAIT_READY_ATTEMPTS=120 ./scripts/wait-ready.sh http://127.0.0.1:18081/readyz

npm --prefix console run test:e2e -- e2e/real-backend-tutorial-clone.spec.ts
find "$private_output/tutorial-experiments" -type f -size +0c -print -quit | grep -q .
