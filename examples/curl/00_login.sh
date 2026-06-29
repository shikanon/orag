#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin}"

response="$(curl -sS "$BASE_URL/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$(json_escape "$ADMIN_USERNAME")\",\"password\":\"$(json_escape "$ADMIN_PASSWORD")\"}")"

token="$(printf '%s\n' "$response" | extract_json_string access_token)"
if [ "$token" = "" ]; then
  printf '%s\n' "$response" >&2
  exit 1
fi

umask 077
printf '%s\n' "$token" > "$TOKEN_FILE"
printf '%s\n' "$response"
