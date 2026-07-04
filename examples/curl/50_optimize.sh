#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"

if [ "${DATASET_ID:-}" != "" ]; then
  dataset_id="$DATASET_ID"
elif [ -s "$DATASET_ID_FILE" ]; then
  dataset_id="$(cat "$DATASET_ID_FILE")"
else
  printf '%s\n' "missing DATASET_ID; run examples/curl/40_eval.sh first" >&2
  exit 1
fi

PROFILE="${PROFILE:-realtime}"
TOP_KS="${TOP_KS:-5,8}"
MAX_CANDIDATES="${MAX_CANDIDATES:-4}"
OBJECTIVE="${OBJECTIVE:-pairwise_accuracy}"
SELECTION_SPLIT="${SELECTION_SPLIT:-eval}"
HOLDOUT_SPLIT="${HOLDOUT_SPLIT:-holdout}"

dense_top_k_json="[$TOP_KS]"
payload='{"dataset_id":"'"$(json_escape "$dataset_id")"'","knowledge_base_id":"'"$(json_escape "$kb_id")"'","profile":"'"$(json_escape "$PROFILE")"'","objective":{"maximize":"'"$(json_escape "$OBJECTIVE")"'"},"search_space":{"retrieval":{"dense_top_k":'"$dense_top_k_json"'}},"search":{"strategy":"grid","max_candidates":'"$MAX_CANDIDATES"'},"selection_split":"'"$(json_escape "$SELECTION_SPLIT")"'","holdout_split":"'"$(json_escape "$HOLDOUT_SPLIT")"'"}'

submit_response="$(request_json POST /v1/optimizations "$payload" "$token")"
run_id="$(printf '%s\n' "$submit_response" | extract_json_string run_id)"
save_if_not_empty "$run_id" "$OPTIMIZATION_ID_FILE"

printf '%s\n' "$submit_response"
if [ "$run_id" = "" ]; then
  exit 1
fi

info "saved optimization id to $OPTIMIZATION_ID_FILE"
info "polling /v1/optimizations/$run_id"
curl -sS "$BASE_URL/v1/optimizations/$run_id" \
  -H "Authorization: Bearer $token"
printf '\n'
