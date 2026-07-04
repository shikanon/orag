#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
DATASET_NAME="${DATASET_NAME:-orag regression}"
DATASET_KIND="${DATASET_KIND:-golden}"
EVAL_QUERY="${EVAL_QUERY:-ORAG 使用什么向量库？}"
GROUND_TRUTH="${GROUND_TRUTH:-Qdrant}"
PROFILE="${PROFILE:-realtime}"
TOP_K="${TOP_K:-8}"

dataset_response="$(curl -sS "$BASE_URL/v1/datasets" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$(json_escape "$DATASET_NAME")\",\"kind\":\"$(json_escape "$DATASET_KIND")\"}")"

dataset_id="$(printf '%s\n' "$dataset_response" | extract_json_string id)"
if [ "$dataset_id" = "" ]; then
  printf '%s\n' "$dataset_response" >&2
  exit 1
fi
printf '%s\n' "$dataset_id" > "$DATASET_ID_FILE"

relevant_doc_ids="[]"
if [ -s "$DOCUMENT_ID_FILE" ]; then
  document_id="$(cat "$DOCUMENT_ID_FILE")"
  relevant_doc_ids="[\"$(json_escape "$document_id")\"]"
fi

request_json POST "/v1/datasets/$dataset_id/items" "{"query":"$(json_escape "$EVAL_QUERY")","ground_truth":"$(json_escape "$GROUND_TRUTH")","relevant_doc_ids":$relevant_doc_ids}" "$token" >/dev/null

eval_response="$(request_json POST /v1/evaluations "{"dataset_id":"$(json_escape "$dataset_id")","knowledge_base_id":"$(json_escape "$kb_id")","profile":"$(json_escape "$PROFILE")","top_k":$TOP_K}" "$token")"
eval_id="$(printf '%s
' "$eval_response" | extract_json_string id)"
save_if_not_empty "$eval_id" "$EVAL_ID_FILE"
[ "$eval_id" = "" ] || info "saved evaluation id to $EVAL_ID_FILE"
info "response metrics include answer_accuracy, pairwise_accuracy as the primary quality metric, and retrieval diagnostics: ndcg_at_k, recall_at_k, mrr, map, coverage, retrieval_failure_rate, redundancy_rate, alpha_ndcg, aspect_coverage"
printf '%s
' "$eval_response"
