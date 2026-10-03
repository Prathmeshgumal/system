#!/usr/bin/env bash
# End-to-end check of full mode: build images, start everything, test through
# Caddy on :8000 exactly as a browser would, then shut down (data is kept).
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=http://localhost:8000
fail() { echo "❌ $*"; exit 1; }
cleanup() { docker compose --profile full down >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "▶ building and starting full mode (waits for healthchecks)…"
VERSION=$(git rev-parse --short HEAD) docker compose --profile full up -d --build --wait

echo "▶ migrate job finished successfully"
code=$(docker compose --profile full ps -a migrate --format '{{.ExitCode}}')
[[ $code == 0 ]] || fail "migrate exited with $code (see: docker compose logs migrate)"

echo "▶ GET /api/status through Caddy"
body=$(curl -fsS "$BASE/api/status") || fail "/api/status did not return 2xx"
echo "  $body"
[[ $body == *'"api":"ok"'* ]] || fail "api is not ok"
[[ $body == *'"postgres":{"status":"ok"'* ]] || fail "postgres is not ok"
[[ $body == *'"db_pool":{'* ]] || fail "db_pool stats missing"

echo "▶ GET /livez through Caddy must NOT reach the API"
livez=$(curl -sS "$BASE/livez")
[[ $livez != *'"alive"'* ]] || fail "/livez is exposed through the proxy"
[[ $livez == *'<div id="root">'* ]] || fail "expected the React page for /livez"

echo "▶ POST /api/notes, then GET /api/notes"
note="smoke test $(date +%s)"
headers=$(mktemp)
created=$(curl -fsS -D "$headers" -X POST "$BASE/api/notes" \
  -H 'Content-Type: application/json' -d "{\"body\":\"$note\"}") || fail "POST /api/notes failed"
echo "  $created"
grep -qi '^HTTP/.* 201' "$headers" || fail "POST /api/notes did not return 201"
grep -qi '^x-request-id: [0-9a-f]\+' "$headers" || fail "X-Request-ID header missing"
curl -fsS "$BASE/api/notes" | grep -q "$note" || fail "created note not in GET /api/notes"

echo "▶ invalid note → 400 validation_failed"
bad=$(curl -sS -o - -w '%{http_code}' -X POST "$BASE/api/notes" -H 'Content-Type: application/json' -d '{"body":""}')
[[ $bad == *'"code":"validation_failed"'*400 ]] || fail "expected 400 validation_failed, got: $bad"

echo "▶ unknown API route → 404 in our error shape"
nf=$(curl -sS -o - -w '%{http_code}' "$BASE/api/does-not-exist")
[[ $nf == *'"code":"not_found"'*404 ]] || fail "expected 404 not_found, got: $nf"

echo "▶ GET / serves the React app"
curl -fsS "$BASE/" | grep -q '<div id="root">' || fail "React app not served"

echo "✅ smoke test passed"
