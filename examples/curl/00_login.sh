#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin}"

response="$(request_json POST /v1/auth/login "{\"username\":\"$(json_escape "$ADMIN_USERNAME")\",\"password\":\"$(json_escape "$ADMIN_PASSWORD")\"}")"

token="$(printf '%s\n' "$response" | extract_json_string access_token)"
if [ "$token" = "" ]; then
  printf '%s\n' "$response" >&2
  die "login response did not contain access_token; verify ADMIN_USERNAME and ADMIN_PASSWORD"
fi

umask 077
printf '%s\n' "$token" > "$TOKEN_FILE"
info "saved bearer token to $TOKEN_FILE"
printf '%s\n' "$response"

