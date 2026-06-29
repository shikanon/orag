#!/usr/bin/env sh
set -eu
url="${1:-http://localhost:8080/readyz}"
attempts="${WAIT_READY_ATTEMPTS:-60}"
for _ in $(seq 1 "$attempts"); do
  if curl -fsS "$url" >/dev/null; then
    exit 0
  fi
  sleep 1
done
echo "service not ready after ${attempts}s: $url" >&2
exit 1
