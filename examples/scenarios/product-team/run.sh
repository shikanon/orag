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

export DOC_NAME="${DOC_NAME:-product-launch-readiness.md}"
export DOC_SOURCE_URI="${DOC_SOURCE_URI:-example://scenarios/product-team/demo-data.md}"
export DOC_CONTENT="${DOC_CONTENT:-$(cat "$DEMO_DATA_FILE")}"
export QUERY="${QUERY:-Is the onboarding assistant ready to launch, and what evidence should product review?}"
export DATASET_NAME="${DATASET_NAME:-product launch readiness}"
export EVAL_QUERY="${EVAL_QUERY:-What evidence should product review before launching an ORAG assistant?}"
export GROUND_TRUTH="${GROUND_TRUTH:-Review grounded answers, citations, trace IDs, evaluation metrics, and selected optimization candidates before launch.}"

printf '%s\n' "Running product team ORAG demo..."
./examples/curl/05_health_ready.sh
./examples/curl/00_login.sh
./examples/curl/10_create_kb.sh
./examples/curl/20_upload_doc.sh
./examples/curl/30_query.sh
./examples/curl/40_eval.sh
./examples/curl/45_optimize.sh
