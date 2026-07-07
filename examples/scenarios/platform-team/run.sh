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

export DOC_NAME="${DOC_NAME:-platform-rag-service-onboarding.md}"
export DOC_SOURCE_URI="${DOC_SOURCE_URI:-example://scenarios/platform-team/demo-data.md}"
export DOC_CONTENT="${DOC_CONTENT:-$(cat "$DEMO_DATA_FILE")}"
export QUERY="${QUERY:-Which ORAG checks should a platform team run before offering RAG as a shared service?}"
export DATASET_NAME="${DATASET_NAME:-platform rag service readiness}"
export EVAL_QUERY="${EVAL_QUERY:-Which checks prove ORAG is ready as a shared RAG service?}"
export GROUND_TRUTH="${GROUND_TRUTH:-Run health readiness, auth, knowledge-base ingestion, query, trace lookup, evaluation, optimization, and agent asset sync checks.}"

printf '%s\n' "Running platform team ORAG demo..."
./examples/curl/05_health_ready.sh
./examples/curl/00_login.sh
./examples/curl/10_create_kb.sh
./examples/curl/20_upload_doc.sh
./examples/curl/30_query.sh
./examples/curl/36_trace_lookup.sh
./examples/curl/40_eval.sh
./examples/curl/45_optimize.sh
GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.4}" CGO_ENABLED="${CGO_ENABLED:-0}" GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}" make agent-sync-check
