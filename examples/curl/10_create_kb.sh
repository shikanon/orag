#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
KB_NAME="${KB_NAME:-Demo KB}"
KB_DESCRIPTION="${KB_DESCRIPTION:-local demo}"

response="$(request_json POST /v1/knowledge-bases "{\"name\":\"$(json_escape "$KB_NAME")\",\"description\":\"$(json_escape "$KB_DESCRIPTION")\"}" "$token")"

kb_id="$(printf '%s\n' "$response" | extract_json_string id)"
if [ "$kb_id" = "" ]; then
  printf '%s\n' "$response" >&2
  die "knowledge base response did not contain id"
fi

printf '%s\n' "$kb_id" > "$KB_ID_FILE"
info "saved knowledge base id to $KB_ID_FILE"
printf '%s\n' "$response"

