#!/usr/bin/env bash
# Render every .d2 file in diagrams/ to an .svg next to it, using the sketch renderer.
# Usage: ./render.sh            (render all)
#        ./render.sh --watch f  (live-reload one diagram in the browser while editing)
set -euo pipefail
cd "$(dirname "$0")/diagrams"

if [[ "${1:-}" == "--watch" ]]; then
  exec d2 --sketch --watch "$2"
fi

for f in *.d2; do
  d2 --sketch --theme=0 --dark-theme=200 "$f" "${f%.d2}.svg"
done
