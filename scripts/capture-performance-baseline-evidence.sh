#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

usage() {
  echo "usage: $0 --output DIR --build-revision FULL_GIT_REVISION" >&2
  exit 2
}

output=""
build_revision=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) output=${2:-}; shift 2 ;;
    --build-revision) build_revision=${2:-}; shift 2 ;;
    *) usage ;;
  esac
done

[[ -n "$output" && -n "$build_revision" ]] || usage
for command in go jq shasum git; do
  command -v "$command" >/dev/null || { echo "performance baseline capture requires $command" >&2; exit 2; }
done
[[ "$build_revision" =~ ^[0-9a-f]{40}$ ]] || { echo "build revision must be a full lowercase Git SHA" >&2; exit 2; }
[[ "$(git rev-parse "$build_revision^{commit}")" == "$build_revision" ]] || { echo "build revision does not resolve to a commit" >&2; exit 2; }
[[ ! -e "$output" ]] || { echo "output directory already exists: $output" >&2; exit 2; }

if [[ "$(uname -s)" == "Darwin" ]]; then
  os_name="macOS"
  os_version="$(sw_vers -productVersion)"
  os_build="$(sw_vers -buildVersion)"
  cpu_model="$(sysctl -n machdep.cpu.brand_string)"
  memory_bytes="$(sysctl -n hw.memsize)"
  host_arch="$(uname -m)"
else
  os_name="$(uname -s)"
  os_version="$(uname -r)"
  os_build="unknown"
  cpu_model="$(LC_ALL=C lscpu | awk -F: '/Model name/ {gsub(/^ +/, "", $2); print $2; exit}')"
  memory_bytes="$(awk '/MemTotal/ {print $2 * 1024; exit}' /proc/meminfo)"
  host_arch="$(uname -m)"
fi

[[ -n "$cpu_model" && "$memory_bytes" =~ ^[0-9]+$ ]] || { echo "unable to collect allowlisted machine disclosure" >&2; exit 2; }

mkdir -p "$output"
CGO_ENABLED=0 GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}" go run ./cmd/oragctl benchmark-run \
  --output "$output/report.json" \
  --build-revision "$build_revision"

runner_command="CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/oragctl benchmark-run --output report.json --build-revision $build_revision"
jq -n \
  --arg captured_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg cpu_model "$cpu_model" \
  --argjson memory_bytes "$memory_bytes" \
  --arg host_arch "$host_arch" \
  --arg go_version "$(go version)" \
  --arg goos "$(go env GOOS)" \
  --arg goarch "$(go env GOARCH)" \
  --arg os_name "$os_name" \
  --arg os_version "$os_version" \
  --arg os_build "$os_build" \
  --arg runner_command "$runner_command" \
  '{schema_version:"orag.performance-baseline-environment.v1",captured_at:$captured_at,machine:{cpu_model:$cpu_model,memory_bytes:$memory_bytes,host_arch:$host_arch},runtime:{go_version:$go_version,goos:$goos,goarch:$goarch},operating_system:{name:$os_name,version:$os_version,build:$os_build},runner_command:$runner_command,scope:"local deterministic-mock regression evidence; not a production or cross-hardware claim"}' \
  >"$output/environment.json"

report_sha256="$(shasum -a 256 "$output/report.json" | awk '{print $1}')"
evidence_id="$(basename "$output")"
jq -n \
  --arg id "$evidence_id" \
  --arg build_revision "$build_revision" \
  --arg report_fingerprint_sha256 "$report_sha256" \
  '{schema_version:"orag.performance-baseline-evidence.v1",id:$id,build_revision:$build_revision,report_fingerprint_sha256:$report_fingerprint_sha256,artifacts:["report.json","environment.json","manifest.json"]}' \
  >"$output/manifest.json"
(cd "$output" && shasum -a 256 report.json environment.json manifest.json >SHA256SUMS)
"$root/scripts/verify-performance-baseline-evidence.sh" --dir "$output"
echo "captured public performance baseline evidence: $output"
