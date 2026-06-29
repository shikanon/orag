#!/usr/bin/env sh

SCRIPT_DIR="$(CDPATH= cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd "$SCRIPT_DIR/../.." && pwd)"

BASE_URL="${BASE_URL:-http://localhost:8080}"
STATE_DIR="${STATE_DIR:-$REPO_ROOT/.orag-demo}"
TOKEN_FILE="$STATE_DIR/token"
KB_ID_FILE="$STATE_DIR/kb_id"
DATASET_ID_FILE="$STATE_DIR/dataset_id"
DOCUMENT_ID_FILE="$STATE_DIR/document_id"
JOB_ID_FILE="$STATE_DIR/job_id"

mkdir -p "$STATE_DIR"

extract_json_string() {
  key="$1"
  sed -n "s/.*\"$key\":\"\([^\"]*\)\".*/\1/p"
}

json_escape() {
  printf '%s' "$1" | tr '\n' ' ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

get_token() {
  if [ "${TOKEN:-}" != "" ]; then
    printf '%s\n' "$TOKEN"
    return
  fi
  if [ -s "$TOKEN_FILE" ]; then
    cat "$TOKEN_FILE"
    return
  fi
  response="$("$SCRIPT_DIR/00_login.sh")"
  token="$(printf '%s\n' "$response" | extract_json_string access_token)"
  if [ "$token" = "" ]; then
    printf '%s\n' "failed to get token from login response" >&2
    exit 1
  fi
  printf '%s\n' "$token"
}

get_kb_id() {
  if [ "${KB_ID:-}" != "" ]; then
    printf '%s\n' "$KB_ID"
    return
  fi
  if [ -s "$KB_ID_FILE" ]; then
    cat "$KB_ID_FILE"
    return
  fi
  printf '%s\n' "missing KB_ID; run examples/curl/10_create_kb.sh first" >&2
  exit 1
}
