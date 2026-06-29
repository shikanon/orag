#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
QUERY="${QUERY:-ORAG 支持哪些检索能力？}"
PROFILE="${PROFILE:-realtime}"

curl -sS "$BASE_URL/v1/query" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "{\"knowledge_base_id\":\"$(json_escape "$kb_id")\",\"query\":\"$(json_escape "$QUERY")\",\"profile\":\"$(json_escape "$PROFILE")\"}"
