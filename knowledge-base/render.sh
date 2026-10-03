#!/usr/bin/env bash
# Render every .d2 file under knowledge-base/ to an .svg next to it, using the sketch renderer.
# Usage: ./render.sh                 (render all; lists any failures and exits 1)
#        ./render.sh --watch <file>  (live-reload one diagram in the browser while editing)
set -uo pipefail
cd "$(dirname "$0")"

if [[ "${1:-}" == "--watch" ]]; then
  exec d2 --sketch --watch "$2"
fi

failed=()
while IFS= read -r -d '' f; do
  if ! out=$(d2 --sketch --theme=0 --dark-theme=200 "$f" "${f%.d2}.svg" 2>&1); then
    failed+=("$f")
    echo "$out" | grep -i 'err' >&2
  fi
done < <(find . -name '*.d2' -print0)

if ((${#failed[@]})); then
  echo "❌ ${#failed[@]} diagram(s) failed: ${failed[*]}" >&2
  exit 1
fi
echo "✅ all diagrams rendered"
