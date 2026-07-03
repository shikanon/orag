#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
TRACE_LIMIT="${TRACE_LIMIT:-10}"

info "listing recent traces from /v1/traces"
list_response="$(request_get "/v1/traces?limit=$TRACE_LIMIT" "$token")"
printf '%s\n' "$list_response"

if [ "${TRACE_ID:-}" != "" ] || [ -s "$TRACE_ID_FILE" ]; then
  trace_id="$(get_trace_id)"
else
  trace_id="$(printf '%s\n' "$list_response" | extract_json_string trace_id | head -n 1)"
fi

if [ "$trace_id" = "" ]; then
  die "no trace id available; run examples/curl/30_query.sh or examples/curl/35_query_stream.sh first"
fi

save_if_not_empty "$trace_id" "$TRACE_ID_FILE"
info "fetching trace detail from /v1/traces/$trace_id"
request_get "/v1/traces/$trace_id" "$token"

