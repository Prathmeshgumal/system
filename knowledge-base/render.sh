#!/usr/bin/env bash
# Render every .d2 file under knowledge-base/ to an .svg next to it, using the sketch renderer.
# Usage: ./render.sh                 (render all)
#        ./render.sh --watch <file>  (live-reload one diagram in the browser while editing)
set -euo pipefail
cd "$(dirname "$0")"

if [[ "${1:-}" == "--watch" ]]; then
  exec d2 --sketch --watch "$2"
fi

find . -name '*.d2' -print0 | while IFS= read -r -d '' f; do
  d2 --sketch --theme=0 --dark-theme=200 "$f" "${f%.d2}.svg"
done
