#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

token="$(get_token)"
kb_id="$(get_kb_id)"
require_curl

UPLOAD_PATH="${UPLOAD_PATH:-$UPLOAD_FILE}"
if [ ! -s "$UPLOAD_PATH" ]; then
  cat > "$UPLOAD_PATH" <<'EOF'
ORAG service-mode multipart upload example.
This file is generated under .orag-demo and uploaded through the public HTTP API.
EOF
  info "created sample upload file at $UPLOAD_PATH"
fi

tmp="${TMPDIR:-/tmp}/orag-example-response.$$"
code="$(curl -sS -o "$tmp" -w '%{http_code}' "$BASE_URL/v1/knowledge-bases/$kb_id/documents" \
  -H "Authorization: Bearer $token" \
  -F "file=@$UPLOAD_PATH")" || {
  rm -f "$tmp"
  die "cannot reach ORAG service at $BASE_URL; start it with scripts/dev-up.sh and make run"
}
response="$(cat "$tmp")"
rm -f "$tmp"
case "$code" in
  2*) ;;
  *)
    printf '%s\n' "$response" >&2
    die "POST /v1/knowledge-bases/{id}/documents failed with HTTP $code; verify UPLOAD_PATH and ingestion dependencies"
    ;;
esac

document_id="$(printf '%s\n' "$response" | extract_document_id)"
job_id="$(printf '%s\n' "$response" | extract_job_id)"
save_if_not_empty "$document_id" "$DOCUMENT_ID_FILE"
save_if_not_empty "$job_id" "$JOB_ID_FILE"
[ "$document_id" = "" ] || info "saved document id to $DOCUMENT_ID_FILE"
[ "$job_id" = "" ] || info "saved ingestion job id to $JOB_ID_FILE"
printf '%s\n' "$response"

