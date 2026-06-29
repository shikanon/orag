#!/usr/bin/env sh
set -eu
if [ "${DEV_DOWN_VOLUMES:-}" = "1" ]; then
  docker compose -f deployments/docker-compose.yml down -v
else
  docker compose -f deployments/docker-compose.yml down
fi
