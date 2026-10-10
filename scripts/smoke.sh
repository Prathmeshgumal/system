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

echo "▶ GET /api/status through Caddy"
body=$(curl -fsS "$BASE/api/status") || fail "/api/status did not return 2xx"
echo "  $body"
[[ $body == *'"api":"ok"'* ]] || fail "api is not ok"
[[ $body == *'"postgres":{"status":"ok"'* ]] || fail "postgres is not ok"

echo "▶ GET /livez through Caddy must NOT reach the API"
livez=$(curl -sS "$BASE/livez")
[[ $livez != *'"alive"'* ]] || fail "/livez is exposed through the proxy"
[[ $livez == *'<div id="root">'* ]] || fail "expected the React page for /livez"

echo "▶ GET / serves the React app"
curl -fsS "$BASE/" | grep -q '<div id="root">' || fail "React app not served"

echo "▶ sign up, then refresh with the cookie — through Caddy, with a browser-like Origin"
jar=$(mktemp)
name="smoke$(date +%s)"
curl -fsS -c "$jar" -H 'Content-Type: application/json' -H "Origin: $BASE" "$BASE/api/auth/signup" \
  -d "{\"email\":\"$name@example.com\",\"username\":\"$name\",\"display_name\":\"Smoke\",\"password\":\"password123\"}" \
  | grep -q '"access_token"' || fail "signup failed"
token=$(curl -fsS -b "$jar" -c "$jar" -X POST -H "Origin: $BASE" "$BASE/api/auth/refresh" \
  | sed -E 's/.*"access_token":"([^"]+)".*/\1/') || fail "refresh failed (Host/Origin rewritten by the proxy?)"
rm -f "$jar"

echo "▶ authenticated API call and a WebSocket ticket"
curl -fsS -H "Authorization: Bearer $token" "$BASE/api/chats" | grep -q '"chats"' || fail "GET /api/chats failed"
ticket=$(curl -fsS -X POST -H "Authorization: Bearer $token" "$BASE/api/ws/ticket" \
  | sed -E 's/.*"ticket":"([^"]+)".*/\1/')

echo "▶ WebSocket upgrade through Caddy"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 --http1.1 \
  -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' -H "Origin: $BASE" \
  "$BASE/ws?ticket=$ticket" || true)
[[ $code == 101 ]] || fail "WebSocket upgrade returned $code, want 101"

echo "✅ smoke test passed"
