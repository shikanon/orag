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
TRACE_ID_FILE="$STATE_DIR/trace_id"
EVAL_ID_FILE="$STATE_DIR/evaluation_id"
UPLOAD_FILE="$STATE_DIR/upload.md"

mkdir -p "$STATE_DIR"

info() {
  printf '%s\n' "[orag-example] $*" >&2
}

die() {
  printf '%s\n' "[orag-example] ERROR: $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing dependency '$1'; install it and retry"
}

require_curl() {
  require_command curl
}

extract_json_string() {
  key="$1"
  sed -n "s/.*\"$key\":\"\([^\"]*\)\".*/\1/p"
}

extract_document_id() {
  sed -n 's/.*"document":{[^}]*"id":"\([^"]*\)".*/\1/p'
}

extract_job_id() {
  sed -n 's/.*"job":{[^}]*"id":"\([^"]*\)".*/\1/p'
}

json_escape() {
  printf '%s' "$1" | tr '\n' ' ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

require_state_file() {
  label="$1"
  file="$2"
  hint="$3"
  if [ ! -s "$file" ]; then
    die "missing $label at $file; $hint"
  fi
}

get_token() {
  if [ "${TOKEN:-}" != "" ]; then
    printf '%s\n' "$TOKEN"
    return
  fi
  require_state_file "bearer token" "$TOKEN_FILE" "run examples/curl/00_login.sh or export TOKEN"
  cat "$TOKEN_FILE"
}

get_kb_id() {
  if [ "${KB_ID:-}" != "" ]; then
    printf '%s\n' "$KB_ID"
    return
  fi
  require_state_file "knowledge base id" "$KB_ID_FILE" "run examples/curl/10_create_kb.sh or export KB_ID"
  cat "$KB_ID_FILE"
}

get_dataset_id() {
  if [ "${DATASET_ID:-}" != "" ]; then
    printf '%s\n' "$DATASET_ID"
    return
  fi
  require_state_file "dataset id" "$DATASET_ID_FILE" "run examples/curl/40_eval.sh or export DATASET_ID"
  cat "$DATASET_ID_FILE"
}

get_trace_id() {
  if [ "${TRACE_ID:-}" != "" ]; then
    printf '%s\n' "$TRACE_ID"
    return
  fi
  require_state_file "trace id" "$TRACE_ID_FILE" "run examples/curl/30_query.sh or examples/curl/35_query_stream.sh, or export TRACE_ID"
  cat "$TRACE_ID_FILE"
}

save_if_not_empty() {
  value="$1"
  file="$2"
  if [ "$value" != "" ]; then
    printf '%s\n' "$value" > "$file"
  fi
}

request_get() {
  path="$1"
  token="${2:-}"
  require_curl
  tmp="${TMPDIR:-/tmp}/orag-example-response.$$"
  if [ "$token" != "" ]; then
    code="$(curl -sS -o "$tmp" -w '%{http_code}' "$BASE_URL$path" \
      -H "Authorization: Bearer $token")" || {
      rm -f "$tmp"
      die "cannot reach ORAG service at $BASE_URL; start it with scripts/dev-up.sh and make run"
    }
  else
    code="$(curl -sS -o "$tmp" -w '%{http_code}' "$BASE_URL$path")" || {
      rm -f "$tmp"
      die "cannot reach ORAG service at $BASE_URL; start it with scripts/dev-up.sh and make run"
    }
  fi
  response="$(cat "$tmp")"
  rm -f "$tmp"
  case "$code" in
    2*) printf '%s\n' "$response" ;;
    *)
      printf '%s\n' "$response" >&2
      die "GET $path failed with HTTP $code; check BASE_URL, token, and service dependencies"
      ;;
  esac
}

request_json() {
  method="$1"
  path="$2"
  body="$3"
  token="${4:-}"
  require_curl
  tmp="${TMPDIR:-/tmp}/orag-example-response.$$"
  if [ "$token" != "" ]; then
    code="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$BASE_URL$path" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      -d "$body")" || {
      rm -f "$tmp"
      die "cannot reach ORAG service at $BASE_URL; start it with scripts/dev-up.sh and make run"
    }
  else
    code="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$BASE_URL$path" \
      -H "Content-Type: application/json" \
      -d "$body")" || {
      rm -f "$tmp"
      die "cannot reach ORAG service at $BASE_URL; start it with scripts/dev-up.sh and make run"
    }
  fi
  response="$(cat "$tmp")"
  rm -f "$tmp"
  case "$code" in
    2*) printf '%s\n' "$response" ;;
    *)
      printf '%s\n' "$response" >&2
      die "$method $path failed with HTTP $code; check request body, token, and service dependencies"
      ;;
  esac
}

