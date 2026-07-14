#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$ROOT/deployments/docker-compose.yml"
PROJECT_DIR="$ROOT/.orag-demo"
TIMEOUT_SECONDS="${ORAG_DEMO_WAIT_SECONDS:-240}"

# Avoid colliding with common host-native development services while keeping
# standard container ports inside the Compose network.
export POSTGRES_PORT="${POSTGRES_PORT:-55432}"
export QDRANT_HTTP_PORT="${QDRANT_HTTP_PORT:-56333}"
export QDRANT_GRPC_PORT="${QDRANT_GRPC_PORT:-56334}"

compose() {
  docker compose -f "$COMPOSE_FILE" --profile demo "$@"
}

compose up --build -d

container_id="$(compose ps -a -q demo)"
if [ -z "$container_id" ]; then
  compose logs --no-color demo >&2
  echo "demo container was not created" >&2
  exit 1
fi

elapsed=0
while [ "$(docker inspect -f '{{.State.Running}}' "$container_id")" = "true" ]; do
  if [ "$elapsed" -ge "$TIMEOUT_SECONDS" ]; then
    compose logs --no-color demo >&2
    echo "demo did not finish within ${TIMEOUT_SECONDS}s" >&2
    exit 1
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

exit_code="$(docker inspect -f '{{.State.ExitCode}}' "$container_id")"
if [ "$exit_code" -ne 0 ]; then
  compose logs --no-color demo >&2
  exit "$exit_code"
fi

console_id="$(compose ps -q orag-console)"
elapsed=0
while [ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$console_id")" != "healthy" ]; do
  if [ "$elapsed" -ge "$TIMEOUT_SECONDS" ]; then
    compose logs --no-color orag-console >&2
    echo "console did not become healthy within ${TIMEOUT_SECONDS}s" >&2
    exit 1
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

mkdir -p "$PROJECT_DIR"
compose run --rm --no-deps --entrypoint cat demo /demo/walkthrough.json > "$PROJECT_DIR/walkthrough.json"
grep -q '"status": "completed"' "$PROJECT_DIR/walkthrough.json"

echo "ORAG Console: http://localhost:3000"
echo "ORAG API docs: http://localhost:8080/docs"
echo "Walkthrough summary: $PROJECT_DIR/walkthrough.json"
cat "$PROJECT_DIR/walkthrough.json"
