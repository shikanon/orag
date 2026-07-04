#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
QUERY="${QUERY:-Stream a short answer about ORAG retrieval.}"
PROFILE="${PROFILE:-realtime}"
STREAM_MAX_TIME="${STREAM_MAX_TIME:-30}"
require_curl

body="{\"knowledge_base_id\":\"$(json_escape "$kb_id")\",\"query\":\"$(json_escape "$QUERY")\",\"profile\":\"$(json_escape "$PROFILE")\"}"
tmp="${TMPDIR:-/tmp}/orag-example-stream.$$"
code="$(curl -sS --no-buffer --max-time "$STREAM_MAX_TIME" -o "$tmp" -w '%{http_code}' \
  "$BASE_URL/v1/query:stream" \
  -H "Authorization: Bearer $token" \
  -H "Accept: text/event-stream" \
  -H "Content-Type: application/json" \
  -d "$body")" || {
  rm -f "$tmp"
  die "SSE request failed; verify the service is running and STREAM_MAX_TIME is large enough"
}
response="$(cat "$tmp")"
rm -f "$tmp"
case "$code" in
  2*) ;;
  *)
    printf '%s\n' "$response" >&2
    die "POST /v1/query:stream failed with HTTP $code; check token, knowledge base id, and query dependencies"
    ;;
esac
trace_id="$(printf '%s\n' "$response" | extract_json_string trace_id | head -n 1)"
save_if_not_empty "$trace_id" "$TRACE_ID_FILE"
[ "$trace_id" = "" ] || info "saved stream trace id to $TRACE_ID_FILE"
printf '%s\n' "$response"

