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

TOOLCHAIN="${GOTOOLCHAIN:-go1.26.4}"
FLAGS="${GOFLAGS:--tags=stdjson,gjson}"

printf '%s\n' "Running agent developer ORAG demo..."
printf '%s\n' "Demo data: $DEMO_DATA_FILE"
head -n 2 examples/mcp/ralph-loop-stdio-smoke.jsonl \
| GOTOOLCHAIN="$TOOLCHAIN" CGO_ENABLED="${CGO_ENABLED:-0}" GOFLAGS="$FLAGS" go run ./cmd/orag-mcp --openapi api/openapi.yaml
GOTOOLCHAIN="$TOOLCHAIN" CGO_ENABLED="${CGO_ENABLED:-0}" GOFLAGS="$FLAGS" make mcp-self-check-smoke
GOTOOLCHAIN="$TOOLCHAIN" CGO_ENABLED="${CGO_ENABLED:-0}" GOFLAGS="$FLAGS" make agent-sync-check
