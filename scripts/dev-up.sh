#!/usr/bin/env sh
set -eu
docker compose -f deployments/docker-compose.yml up -d postgres qdrant
printf '%s\n' "PostgreSQL and Qdrant are starting."
printf '%s\n' "Next: make migrate && make run"
