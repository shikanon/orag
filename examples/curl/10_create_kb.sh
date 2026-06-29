#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
KB_NAME="${KB_NAME:-Demo KB}"
KB_DESCRIPTION="${KB_DESCRIPTION:-local demo}"

response="$(curl -sS "$BASE_URL/v1/knowledge-bases" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$(json_escape "$KB_NAME")\",\"description\":\"$(json_escape "$KB_DESCRIPTION")\"}")"

kb_id="$(printf '%s\n' "$response" | extract_json_string id)"
if [ "$kb_id" = "" ]; then
  printf '%s\n' "$response" >&2
  exit 1
fi

printf '%s\n' "$kb_id" > "$KB_ID_FILE"
printf '%s\n' "$response"
