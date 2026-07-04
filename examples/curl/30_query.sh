#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
QUERY="${QUERY:-What retrieval capabilities does ORAG support?}"
PROFILE="${PROFILE:-realtime}"

response="$(request_json POST /v1/query "{\"knowledge_base_id\":\"$(json_escape "$kb_id")\",\"query\":\"$(json_escape "$QUERY")\",\"profile\":\"$(json_escape "$PROFILE")\"}" "$token")"
trace_id="$(printf '%s\n' "$response" | extract_json_string trace_id)"
save_if_not_empty "$trace_id" "$TRACE_ID_FILE"
[ "$trace_id" = "" ] || info "saved trace id to $TRACE_ID_FILE"
printf '%s\n' "$response"

