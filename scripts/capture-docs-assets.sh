#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output="$repo_root/docs-site/screenshots"
frames="$(mktemp -d)"
trap 'rm -rf "$frames"' EXIT

mkdir -p "$output"

playwright="$repo_root/console/node_modules/.bin/playwright"
if [[ ! -x "$playwright" ]]; then
  echo "Playwright is missing; run npm --prefix console ci first." >&2
  exit 1
fi

"$playwright" screenshot --viewport-size="1440,1000" "http://127.0.0.1:8080/docs" "$output/api-reference.png"
"$playwright" screenshot --viewport-size="1440,1000" --full-page "http://127.0.0.1:4173/" "$output/hosted-docs-home.png"

"$playwright" screenshot --viewport-size="1440,900" "http://127.0.0.1:4173/" "$frames/01.png"
"$playwright" screenshot --viewport-size="1440,900" "http://127.0.0.1:4173/#quickstart" "$frames/02.png"
"$playwright" screenshot --viewport-size="1440,900" "http://127.0.0.1:8080/docs" "$frames/03.png"

ffmpeg -loglevel error -y -framerate 0.4 -i "$frames/%02d.png" -vf "fps=10,scale=1200:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer" "$output/walkthrough.gif"
echo "Captured documentation assets in $output"
