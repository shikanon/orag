#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

usage() {
  echo "usage: $0 --dir EVIDENCE_DIR" >&2
  exit 2
}

dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) dir=${2:-}; shift 2 ;;
    *) usage ;;
  esac
done

[[ -n "$dir" ]] || usage
for command in go jq shasum; do
  command -v "$command" >/dev/null || { echo "performance baseline verifier requires $command" >&2; exit 2; }
done
for artifact in report.json environment.json manifest.json SHA256SUMS; do
  [[ -f "$dir/$artifact" ]] || { echo "missing evidence artifact: $artifact" >&2; exit 1; }
done

(cd "$dir" && shasum -a 256 -c SHA256SUMS)

manifest_keys='["artifacts","build_revision","id","report_fingerprint_sha256","schema_version"]'
environment_keys='["captured_at","machine","operating_system","runner_command","runtime","schema_version","scope"]'
jq -e --argjson expected "$manifest_keys" '
  (keys | sort) == ($expected | sort) and
  .schema_version == "orag.performance-baseline-evidence.v1" and
  (.id | test("^[a-z0-9][a-z0-9-]*$")) and
  (.build_revision | test("^[0-9a-f]{40}$")) and
  (.report_fingerprint_sha256 | test("^[0-9a-f]{64}$")) and
  .artifacts == ["report.json", "environment.json", "manifest.json"]
' "$dir/manifest.json" >/dev/null
jq -e --argjson expected "$environment_keys" '
  (keys | sort) == ($expected | sort) and
  .schema_version == "orag.performance-baseline-environment.v1" and
  (.captured_at | test("Z$")) and
  (.machine | keys | sort) == ["cpu_model", "host_arch", "memory_bytes"] and
  (.machine.cpu_model | type == "string" and length > 0) and
  (.machine.memory_bytes | type == "number" and . > 0) and
  (.machine.host_arch | type == "string" and length > 0) and
  (.runtime | keys | sort) == ["go_version", "goarch", "goos"] and
  (.operating_system | keys | sort) == ["build", "name", "version"] and
  (.runner_command | contains("benchmark-run")) and
  (.scope | contains("not a production or cross-hardware claim"))
' "$dir/environment.json" >/dev/null

report_sha256="$(shasum -a 256 "$dir/report.json" | awk '{print $1}')"
jq -e --arg report_sha256 "$report_sha256" '
  .report_fingerprint_sha256 == $report_sha256
' "$dir/manifest.json" >/dev/null
report_revision="$(jq -er '.provenance.build_revision' "$dir/report.json")"
manifest_revision="$(jq -er '.build_revision' "$dir/manifest.json")"
[[ "$report_revision" == "$manifest_revision" ]] || { echo "report build revision does not match manifest" >&2; exit 1; }
jq -e '.provenance.deterministic_mock == true' "$dir/report.json" >/dev/null
CGO_ENABLED=0 GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}" go run ./cmd/oragctl benchmark-report --file "$dir/report.json" >/dev/null
echo "verified public performance baseline evidence: $dir"
