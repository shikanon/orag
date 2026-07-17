#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base_url="${ORAG_TUTORIAL_PACK_PUBLIC_BASE_URL:-https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs}"
pack_id="${ORAG_TUTORIAL_PACK_ID:-text-rag}"
version="${ORAG_TUTORIAL_PACK_VERSION:-1.1.0}"

case "$base_url" in
  https://*) ;;
  *) echo "ORAG_TUTORIAL_PACK_PUBLIC_BASE_URL must be an HTTPS URL" >&2; exit 2 ;;
esac
case "$pack_id" in
  *[!a-z0-9-]*|"") echo "ORAG_TUTORIAL_PACK_ID must contain lowercase letters, digits, and hyphens" >&2; exit 2 ;;
esac
case "$version" in
  *[!0-9.]*|"") echo "ORAG_TUTORIAL_PACK_VERSION must contain digits and dots" >&2; exit 2 ;;
esac

temporary="$(mktemp -d "${TMPDIR:-/tmp}/orag-public-pack.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT
release_root="$temporary/$pack_id/$version"
mkdir -p "$release_root"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "${base_url%/}/$pack_id/$version/SHA256SUMS" \
  -o "$release_root/SHA256SUMS"

cd "$root"
CGO_ENABLED="${CGO_ENABLED:-0}" GOFLAGS="${GOFLAGS:--tags=stdjson,gjson}" \
  go run ./cmd/orag-pack-release -verify-public "$release_root" -public-base-url "$base_url"
