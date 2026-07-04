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
ENABLE_JUDGE="${ENABLE_JUDGE:-false}"
ENABLE_QAG="${ENABLE_QAG:-false}"
JUDGE_PROVIDER="${JUDGE_PROVIDER:-ark}"
JUDGE_MODEL="${JUDGE_MODEL:-doubao-pro}"

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

item_payload='{"query":"'"$(json_escape "$EVAL_QUERY")"'","ground_truth":"'"$(json_escape "$GROUND_TRUTH")"'","relevant_doc_ids":'"$relevant_doc_ids"'}'
request_json POST "/v1/datasets/$dataset_id/items" "$item_payload" "$token" >/dev/null

judge_json=""
if [ "$ENABLE_JUDGE" = "true" ]; then
  judge_json=',"judge":{"provider":"'"$(json_escape "$JUDGE_PROVIDER")"'","model":"'"$(json_escape "$JUDGE_MODEL")"'","metrics":["faithfulness","groundedness","citation_support"],"strict_json":true}'
fi

qag_json=""
if [ "$ENABLE_QAG" = "true" ]; then
  qag_json=',"qag":{"provider":"'"$(json_escape "$JUDGE_PROVIDER")"'","model":"'"$(json_escape "$JUDGE_MODEL")"'","strict_json":true}'
fi

eval_payload='{"dataset_id":"'"$(json_escape "$dataset_id")"'","knowledge_base_id":"'"$(json_escape "$kb_id")"'","profile":"'"$(json_escape "$PROFILE")"'","top_k":'"$TOP_K$judge_json$qag_json"'}'
eval_response="$(request_json POST /v1/evaluations "$eval_payload" "$token")"
eval_id="$(printf '%s\n' "$eval_response" | extract_json_string id)"
save_if_not_empty "$eval_id" "$EVAL_ID_FILE"
[ "$eval_id" = "" ] || info "saved evaluation id to $EVAL_ID_FILE"
info "response metrics include answer_accuracy, pairwise_accuracy as the primary quality metric, optional judge/QAG metrics, and retrieval diagnostics: ndcg_at_k, recall_at_k, mrr, map, coverage, retrieval_failure_rate, redundancy_rate, alpha_ndcg, aspect_coverage"
printf '%s\n' "$eval_response"
