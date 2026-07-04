#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
DOC_NAME="${DOC_NAME:-orag.md}"
DOC_SOURCE_URI="${DOC_SOURCE_URI:-example://orag}"
DOC_CONTENT="${DOC_CONTENT:-ORAG is a Go RAG framework with Qdrant vector retrieval, PostgreSQL sparse retrieval, RRF, Ark rerank, and Doubao generation.}"

response="$(request_json POST "/v1/knowledge-bases/$kb_id/documents:import" "{\"name\":\"$(json_escape "$DOC_NAME")\",\"source_uri\":\"$(json_escape "$DOC_SOURCE_URI")\",\"content\":\"$(json_escape "$DOC_CONTENT")\"}" "$token")"

document_id="$(printf '%s\n' "$response" | extract_document_id)"
job_id="$(printf '%s\n' "$response" | extract_job_id)"
save_if_not_empty "$document_id" "$DOCUMENT_ID_FILE"
save_if_not_empty "$job_id" "$JOB_ID_FILE"
[ "$document_id" = "" ] || info "saved document id to $DOCUMENT_ID_FILE"
[ "$job_id" = "" ] || info "saved ingestion job id to $JOB_ID_FILE"
printf '%s\n' "$response"

