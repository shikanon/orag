#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
dataset_id="$(get_dataset_id)"
PROFILES_JSON="${PROFILES_JSON:-[\"realtime\",\"high_precision\"]}"
TOP_KS_JSON="${TOP_KS_JSON:-[4,8]}"

request_json POST /v1/optimizations "{\"dataset_id\":\"$(json_escape "$dataset_id")\",\"knowledge_base_id\":\"$(json_escape "$kb_id")\",\"profiles\":$PROFILES_JSON,\"top_ks\":$TOP_KS_JSON}" "$token"

