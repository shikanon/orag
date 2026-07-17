#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

for command in docker curl jq tar shasum go; do
  command -v "$command" >/dev/null || { echo "backup restore drill requires $command" >&2; exit 2; }
done

tmp_root="$root/.tmp/backup-restore-drill"
mkdir -p "$tmp_root"
tmp="$(mktemp -d "$tmp_root/run.XXXXXX")"
backup="$tmp/backup"
snapshots="$tmp/snapshots"
source_project="orag-backup-drill-source"
target_project="orag-backup-drill-target"
source_api_port=18085
target_api_port=18086
source_pg_port=55433
target_pg_port=55434
source_qdrant_http_port=6635
source_qdrant_grpc_port=6636
target_qdrant_http_port=7635
target_qdrant_grpc_port=7636
source_api_pid=""
target_api_pid=""

source_compose=(env POSTGRES_HOST_PORT="$source_pg_port" QDRANT_HTTP_HOST_PORT="$source_qdrant_http_port" QDRANT_GRPC_HOST_PORT="$source_qdrant_grpc_port" docker compose -p "$source_project" -f deployments/docker-compose.restore-drill.yml)
target_compose=(env POSTGRES_HOST_PORT="$target_pg_port" QDRANT_HTTP_HOST_PORT="$target_qdrant_http_port" QDRANT_GRPC_HOST_PORT="$target_qdrant_grpc_port" docker compose -p "$target_project" -f deployments/docker-compose.restore-drill.yml)

cleanup() {
  local status=$?
  for pid in "$source_api_pid" "$target_api_pid"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  "${source_compose[@]}" logs --no-color >"$tmp/source-dependencies.log" 2>&1 || true
  "${target_compose[@]}" logs --no-color >"$tmp/target-dependencies.log" 2>&1 || true
  "${source_compose[@]}" down -v >/dev/null 2>&1 || true
  "${target_compose[@]}" down -v >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

mkdir -p "$backup" "$snapshots"
go build -o "$tmp/orag-api" ./cmd/orag-api

configure_runtime() {
  local pg_port=$1 qdrant_grpc_port=$2 api_port=$3 suffix=$4
  export CGO_ENABLED=0
  export GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}"
  export STORAGE_BACKEND=qdrant_postgres
  export DATABASE_URL="postgres://orag:orag@127.0.0.1:${pg_port}/orag?sslmode=disable"
  export QDRANT_HOST=127.0.0.1
  export QDRANT_GRPC_PORT="$qdrant_grpc_port"
  export QDRANT_COLLECTION="orag_backup_drill_${suffix}_chunks"
  export QDRANT_SEMANTIC_CACHE_COLLECTION="orag_backup_drill_${suffix}_semantic_cache"
  export QDRANT_AUTO_CREATE_COLLECTIONS=true
  export JWT_SECRET="orag-backup-drill-${suffix}-jwt-secret"
  export API_KEY_PEPPER="orag-backup-drill-${suffix}-api-key-pepper"
  export ADMIN_DEFAULT_USERNAME=backup-drill-admin
  export ADMIN_DEFAULT_PASSWORD=backup-drill-password
  export ALLOW_DETERMINISTIC_MOCK=true
  export LLM_CHAT_PROVIDER=mock
  export LLM_EMBEDDING_PROVIDER=mock
  export LLM_RERANK_PROVIDER=mock
  export LLM_MULTIMODAL_PROVIDER=mock
  export PORT="$api_port"
  export OBJECT_STORAGE_PROVIDER=local
  export OBJECT_STORAGE_MOCK_UPLOAD=true
}

"${source_compose[@]}" up -d --wait
configure_runtime "$source_pg_port" "$source_qdrant_grpc_port" "$source_api_port" source
go run ./cmd/oragctl migrate
"$tmp/orag-api" >"$tmp/source-api.log" 2>&1 &
source_api_pid=$!
WAIT_READY_ATTEMPTS=120 ./scripts/wait-ready.sh "http://127.0.0.1:${source_api_port}/readyz"

export ORAG_DEMO_BASE_URL="http://127.0.0.1:${source_api_port}"
export ORAG_DEMO_USERNAME=backup-drill-admin
export ORAG_DEMO_PASSWORD=backup-drill-password
export ORAG_DEMO_SUMMARY="$tmp/source-walkthrough.json"
go run ./cmd/orag-demo

source_kb_id="$(jq -er '.knowledge_base_id' "$tmp/source-walkthrough.json")"
source_trace_id="$(jq -er '.trace_id' "$tmp/source-walkthrough.json")"
source_qdrant_url="http://127.0.0.1:${source_qdrant_http_port}"
collections=("$QDRANT_COLLECTION" "$QDRANT_SEMANTIC_CACHE_COLLECTION")

"${source_compose[@]}" exec -T postgres pg_dump -U orag -d orag --format=custom --no-owner --no-acl >"$backup/postgres.dump"
for collection in "${collections[@]}"; do
  response="$(curl -fsS -X POST "${source_qdrant_url}/collections/${collection}/snapshots")"
  snapshot_name="$(jq -er '.result.name' <<<"$response")"
  curl -fsS "${source_qdrant_url}/collections/${collection}/snapshots/${snapshot_name}" -o "$snapshots/${collection}.snapshot"
done
tar -C "$snapshots" -czf "$backup/qdrant-snapshots.tgz" .

migrations="$(go run ./cmd/oragctl migrate --status | awk '$2 == "applied" {print $1}' | jq -R . | jq -s .)"
build_revision="$(git rev-parse HEAD)"
created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq -n --arg created_at "$created_at" --arg build_revision "$build_revision" --argjson migrations "$migrations" '{schema_version:"orag.backup.v1",created_at:$created_at,build_revision:$build_revision,migrations:$migrations,artifacts:["postgres.dump","qdrant-snapshots.tgz"]}' >"$backup/manifest.json"
(cd "$backup" && shasum -a 256 postgres.dump qdrant-snapshots.tgz >SHA256SUMS)
go run ./cmd/oragctl backup-verify --dir "$backup"
backup_fingerprint="$(shasum -a 256 "$backup/manifest.json" | awk '{print $1}')"

"${target_compose[@]}" up -d --wait
"${target_compose[@]}" exec -T postgres pg_restore -U orag -d orag --no-owner --no-acl --exit-on-error <"$backup/postgres.dump"
restore_snapshots="$tmp/restore-snapshots"
mkdir -p "$restore_snapshots"
tar -C "$restore_snapshots" -xzf "$backup/qdrant-snapshots.tgz"
target_qdrant_url="http://127.0.0.1:${target_qdrant_http_port}"
for collection in "${collections[@]}"; do
  curl -fsS -X POST "${target_qdrant_url}/collections/${collection}/snapshots/upload?priority=snapshot" -F "snapshot=@${restore_snapshots}/${collection}.snapshot" | jq -e '.result == true' >/dev/null
done

configure_runtime "$target_pg_port" "$target_qdrant_grpc_port" "$target_api_port" source
go run ./cmd/oragctl migrate
"$tmp/orag-api" >"$tmp/target-api.log" 2>&1 &
target_api_pid=$!
WAIT_READY_ATTEMPTS=120 ./scripts/wait-ready.sh "http://127.0.0.1:${target_api_port}/readyz"

target_api_url="http://127.0.0.1:${target_api_port}"
token="$(curl -fsS -X POST "${target_api_url}/v1/auth/login" -H 'Content-Type: application/json' --data '{"username":"backup-drill-admin","password":"backup-drill-password"}' | jq -er '.access_token')"
curl -fsS "${target_api_url}/v1/knowledge-bases" -H "Authorization: Bearer ${token}" >"$tmp/target-knowledge-bases.json"
jq -e --arg kb "$source_kb_id" '.items[] | select(.id == $kb)' "$tmp/target-knowledge-bases.json" >/dev/null
query_body="$(jq -n --arg kb "$source_kb_id" '{knowledge_base_id:$kb,query:"What is ORAG and which workflow does it provide?",profile:"realtime"}')"
query_response="$(curl -fsS -X POST "${target_api_url}/v1/query" -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json' --data "$query_body")"
citation_count="$(jq -er '.citations | length | select(. > 0)' <<<"$query_response")"
restored_trace_id="$(jq -er '.trace_id' <<<"$query_response")"
curl -fsS "${target_api_url}/v1/traces/${restored_trace_id}" -H "Authorization: Bearer ${token}" | jq -e --arg trace "$restored_trace_id" '.trace_id == $trace' >/dev/null

jq -n --arg schema_version "orag.backup-restore-drill.v1" --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg build_revision "$build_revision" --arg manifest_sha256 "$backup_fingerprint" --arg source_knowledge_base_id "$source_kb_id" --arg source_trace_id "$source_trace_id" --arg restored_trace_id "$restored_trace_id" --argjson citation_count "$citation_count" '{schema_version:$schema_version,created_at:$created_at,build_revision:$build_revision,backup_manifest_sha256:$manifest_sha256,source_knowledge_base_id:$source_knowledge_base_id,source_trace_id:$source_trace_id,restored_trace_id:$restored_trace_id,citation_count:$citation_count}' >"$tmp/drill-evidence.json"

echo "verified isolated backup restore drill: $tmp/drill-evidence.json"
