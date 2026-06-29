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

curl -sS "$BASE_URL/v1/datasets/$dataset_id/items" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"$(json_escape "$EVAL_QUERY")\",\"ground_truth\":\"$(json_escape "$GROUND_TRUTH")\",\"relevant_doc_ids\":$relevant_doc_ids}" >/dev/null

curl -sS "$BASE_URL/v1/evaluations" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"dataset_id\":\"$(json_escape "$dataset_id")\",\"knowledge_base_id\":\"$(json_escape "$kb_id")\",\"profile\":\"$(json_escape "$PROFILE")\",\"top_k\":$TOP_K}"
