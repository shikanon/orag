#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

for command in docker curl jq go; do
  command -v "$command" >/dev/null || { echo "credential rotation drill requires $command" >&2; exit 2; }
done

tmp_root="$root/.tmp/credential-rotation-drill"
mkdir -p "$tmp_root"
tmp="$(mktemp -d "$tmp_root/run.XXXXXX")"
project="orag-credential-rotation-drill-$RANDOM"
api_port=18087
pg_port=55435
qdrant_http_port=6637
qdrant_grpc_port=6638
api_pid=""
compose=(env POSTGRES_HOST_PORT="$pg_port" QDRANT_HTTP_HOST_PORT="$qdrant_http_port" QDRANT_GRPC_HOST_PORT="$qdrant_grpc_port" docker compose -p "$project" -f deployments/docker-compose.restore-drill.yml)

cleanup() {
  local status=$?
  if [[ -n "$api_pid" ]] && kill -0 "$api_pid" 2>/dev/null; then
    kill "$api_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true
  fi
  "${compose[@]}" logs --no-color >"$tmp/dependencies.log" 2>&1 || true
  "${compose[@]}" down -v >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

go build -o "$tmp/orag-api" ./cmd/orag-api
"${compose[@]}" up -d --wait

export CGO_ENABLED=0
export GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}"
export STORAGE_BACKEND=qdrant_postgres
export DATABASE_URL="postgres://orag:orag@127.0.0.1:${pg_port}/orag?sslmode=disable"
export QDRANT_HOST=127.0.0.1
export QDRANT_GRPC_PORT="$qdrant_grpc_port"
export QDRANT_COLLECTION="orag_credential_rotation_drill_chunks"
export QDRANT_SEMANTIC_CACHE_COLLECTION="orag_credential_rotation_drill_semantic_cache"
export QDRANT_AUTO_CREATE_COLLECTIONS=true
export JWT_SECRET="orag-credential-rotation-drill-jwt-secret"
export API_KEY_PEPPER="orag-credential-rotation-drill-api-key-pepper"
export ADMIN_DEFAULT_USERNAME=credential-drill-admin
export ADMIN_DEFAULT_PASSWORD=credential-drill-password
export ALLOW_DETERMINISTIC_MOCK=true
export LLM_CHAT_PROVIDER=mock
export LLM_EMBEDDING_PROVIDER=mock
export LLM_RERANK_PROVIDER=mock
export LLM_MULTIMODAL_PROVIDER=mock
export OBJECT_STORAGE_PROVIDER=local
export OBJECT_STORAGE_MOCK_UPLOAD=true
export PORT="$api_port"

go run ./cmd/oragctl migrate
"$tmp/orag-api" >"$tmp/api.log" 2>&1 &
api_pid=$!
WAIT_READY_ATTEMPTS=120 ./scripts/wait-ready.sh "http://127.0.0.1:${api_port}/readyz"

api_url="http://127.0.0.1:${api_port}"
token="$(curl -fsS -X POST "${api_url}/v1/auth/login" -H 'Content-Type: application/json' --data '{"username":"credential-drill-admin","password":"credential-drill-password"}' | jq -er '.access_token')"
source_response="$(curl -fsS -X POST "${api_url}/v1/api-keys" -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json' --data '{"name":"credential rotation drill","role":"tenant_admin"}')"
source_id="$(jq -er '.api_key.id' <<<"$source_response")"
source_secret="$(jq -er '.secret' <<<"$source_response")"
replacement_response="$(curl -fsS -X POST "${api_url}/v1/api-keys/${source_id}/rotate" -H "Authorization: Bearer ${token}")"
replacement_id="$(jq -er '.api_key.id' <<<"$replacement_response")"
replacement_source_id="$(jq -er '.api_key.rotated_from_key_id' <<<"$replacement_response")"
replacement_secret="$(jq -er '.secret' <<<"$replacement_response")"
[[ "$replacement_source_id" == "$source_id" ]] || { echo "replacement lineage mismatch" >&2; exit 1; }

source_status="$(curl -sS -o /dev/null -w '%{http_code}' "${api_url}/v1/api-keys" -H "Authorization: Bearer ${source_secret}")"
replacement_status="$(curl -sS -o /dev/null -w '%{http_code}' "${api_url}/v1/api-keys" -H "Authorization: Bearer ${replacement_secret}")"
[[ "$source_status" == "401" ]] || { echo "rotated source status=${source_status}, want 401" >&2; exit 1; }
[[ "$replacement_status" == "200" ]] || { echo "replacement status=${replacement_status}, want 200" >&2; exit 1; }

jq -n \
  --arg schema_version "orag.credential-rotation-drill.v1" \
  --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg build_revision "$(git rev-parse HEAD)" \
  --arg source_key_id "$source_id" \
  --arg replacement_key_id "$replacement_id" \
  --arg source_status "$source_status" \
  --arg replacement_status "$replacement_status" \
  '{schema_version:$schema_version,created_at:$created_at,build_revision:$build_revision,source_key_id:$source_key_id,replacement_key_id:$replacement_key_id,source_http_status:($source_status|tonumber),replacement_http_status:($replacement_status|tonumber),immediate_cutover:true}' \
  >"$tmp/drill-evidence.json"

echo "verified isolated credential rotation drill: $tmp/drill-evidence.json"
