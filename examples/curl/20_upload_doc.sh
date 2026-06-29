#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
DOC_NAME="${DOC_NAME:-orag.md}"
DOC_SOURCE_URI="${DOC_SOURCE_URI:-example://orag}"
DOC_CONTENT="${DOC_CONTENT:-ORAG 是 Go RAG 框架，支持 Qdrant、PostgreSQL sparse retrieval、RRF、Ark Rerank 和豆包生成。}"

response="$(curl -sS "$BASE_URL/v1/knowledge-bases/$kb_id/documents:import" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$(json_escape "$DOC_NAME")\",\"source_uri\":\"$(json_escape "$DOC_SOURCE_URI")\",\"content\":\"$(json_escape "$DOC_CONTENT")\"}")"

document_id="$(printf '%s\n' "$response" | extract_json_string document_id)"
job_id="$(printf '%s\n' "$response" | sed -n 's/.*"job":{"id":"\([^"]*\)".*/\1/p')"
if [ "$document_id" != "" ]; then
  printf '%s\n' "$document_id" > "$DOCUMENT_ID_FILE"
fi
if [ "$job_id" != "" ]; then
  printf '%s\n' "$job_id" > "$JOB_ID_FILE"
fi

printf '%s\n' "$response"
