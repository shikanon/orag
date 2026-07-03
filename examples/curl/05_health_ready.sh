#!/usr/bin/env sh
set -eu

. "$(dirname "$0")/lib.sh"

info "checking process health at $BASE_URL/healthz"
request_get /healthz
info "checking dependency readiness at $BASE_URL/readyz"
request_get /readyz

