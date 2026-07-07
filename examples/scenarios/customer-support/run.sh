#!/usr/bin/env sh
set -eu

SCENARIO_DIR="$(CDPATH= cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd "$SCENARIO_DIR/../../.." && pwd)"
cd "$REPO_ROOT"

DEMO_DATA_FILE="$SCENARIO_DIR/demo-data.md"
if [ ! -s "$DEMO_DATA_FILE" ]; then
  printf '%s\n' "missing demo data: $DEMO_DATA_FILE" >&2
  exit 1
fi

export DOC_NAME="${DOC_NAME:-customer-support-playbook.md}"
export DOC_SOURCE_URI="${DOC_SOURCE_URI:-example://scenarios/customer-support/demo-data.md}"
export DOC_CONTENT="${DOC_CONTENT:-$(cat "$DEMO_DATA_FILE")}"
export UPLOAD_PATH="${UPLOAD_PATH:-$DEMO_DATA_FILE}"
export QUERY="${QUERY:-A customer asks why ORAG answers must include citations and what trace ID should be shared for escalation. How should support reply?}"

printf '%s\n' "Running customer support ORAG demo..."
./examples/curl/05_health_ready.sh
./examples/curl/00_login.sh
./examples/curl/10_create_kb.sh
./examples/curl/20_upload_doc.sh
./examples/curl/25_upload_file.sh
./examples/curl/30_query.sh
./examples/curl/36_trace_lookup.sh
