#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output="${1:-$repo_root/_site}"

rm -rf "$output"
mkdir -p "$output/swagger-ui"
cp -R "$repo_root/docs-site/." "$output/"
cp "$repo_root/api/openapi.yaml" "$output/openapi.yaml"
cp "$repo_root/api/swagger-ui/swagger-ui.css" "$output/swagger-ui/swagger-ui.css"
cp "$repo_root/api/swagger-ui/swagger-ui-bundle.js" "$output/swagger-ui/swagger-ui-bundle.js"
cp "$repo_root/api/swagger-ui/LICENSE" "$output/swagger-ui/LICENSE"
cp "$repo_root/api/swagger-ui/NOTICE" "$output/swagger-ui/NOTICE"

echo "Built hosted documentation in $output"
