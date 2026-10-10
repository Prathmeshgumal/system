#!/usr/bin/env bash
# make deploy — build, ship and start Relay on the server (Part 2.3, ADR 0005).
#
#   1. build the api and web images for linux/amd64, tagged with the git sha
#   2. ship them: docker save | ssh server docker load   (no registry needed)
#   3. copy compose files, Caddyfile, monitoring config and .env (chmod 600)
#   4. docker compose up -d, wait for health checks
#   5. smoke check over HTTPS
#
# Settings come from deploy/.env.prod (DEPLOY_HOST, DEPLOY_USER, DOMAIN, …).
# Roll back:  make deploy RELEASE=<older sha>   (re-ships images you built before)
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "❌ $*" >&2; exit 1; }
step() { echo "▶ $*"; }

ENV_FILE=${ENV_FILE:-deploy/.env.prod}
[[ -f $ENV_FILE ]] || fail "$ENV_FILE not found: cp deploy/.env.prod.example $ENV_FILE and fill it in"
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

HOST=${DEPLOY_HOST:?set DEPLOY_HOST in $ENV_FILE (terraform output public_ip)}
USER_AT_HOST="${DEPLOY_USER:-ubuntu}@$HOST"
SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=10)
[[ -n ${DEPLOY_SSH_KEY:-} ]] && SSH_OPTS+=(-i "${DEPLOY_SSH_KEY/#\~/$HOME}")
REMOTE_DIR=${REMOTE_DIR:-relay}
if [[ -z ${RELEASE:-} ]]; then
  RELEASE=$(git rev-parse --short HEAD)
  [[ -z $(git status --porcelain) ]] || echo "⚠️  uncommitted changes: they're in the images, but not in commit $RELEASE"
fi
remote() { ssh "${SSH_OPTS[@]}" "$USER_AT_HOST" "$@"; }

# Which compose files and profiles the server runs.
COMPOSE=(-f compose.prod.yaml)
[[ ${LOCAL_STORAGE:-false} == true ]] && COMPOSE+=(-f compose.storage.yaml)
[[ ${MONITORING:-true} == true ]] && COMPOSE+=(--profile monitoring)

if [[ ${SKIP_BUILD:-} != 1 ]] && ! docker image inspect "relay-api:$RELEASE" "relay-web:$RELEASE" >/dev/null 2>&1; then
  step "building images for $RELEASE (linux/amd64)"
  docker build --platform linux/amd64 --build-arg VERSION="$RELEASE" -t "relay-api:$RELEASE" backend
  docker build --platform linux/amd64 -t "relay-web:$RELEASE" web
else
  step "images for $RELEASE already built"
fi

step "shipping images to $HOST (only layers matter: ~40 MB compressed)"
docker save "relay-api:$RELEASE" "relay-web:$RELEASE" | gzip -1 | remote 'gunzip | docker load' >/dev/null

step "copying configuration to ~/$REMOTE_DIR"
remote "mkdir -p $REMOTE_DIR/monitoring"
scp "${SSH_OPTS[@]}" -q deploy/compose.prod.yaml deploy/compose.storage.yaml deploy/storage.caddy \
  web/Caddyfile.prod "$USER_AT_HOST:$REMOTE_DIR/"
scp "${SSH_OPTS[@]}" -q -r deploy/monitoring/* "$USER_AT_HOST:$REMOTE_DIR/monitoring/"
# Secrets: readable by the deploy user only, and never in git.
remote "umask 077 && cat > $REMOTE_DIR/.env" <"$ENV_FILE"

step "starting release $RELEASE"
remote "cd $REMOTE_DIR && RELEASE=$RELEASE docker compose ${COMPOSE[*]} up -d --wait --remove-orphans"
remote "cd $REMOTE_DIR && echo $RELEASE > RELEASE && docker image prune -f >/dev/null"

step "smoke check https://$DOMAIN"
CURL=(curl -fsS --max-time 10)
[[ ${SMOKE_INSECURE:-} == 1 ]] && CURL+=(-k) # local test with Caddy's own CA
for i in $(seq 1 30); do # a brand-new certificate can take a few seconds
  if status=$("${CURL[@]}" "https://$DOMAIN/api/status" 2>/dev/null); then
    echo "  $status"
    [[ $status == *'"postgres":{"status":"ok"'* ]] || fail "API is up but the database isn't"
    echo "✅ deployed $RELEASE to https://$DOMAIN"
    exit 0
  fi
  sleep 2
done
fail "https://$DOMAIN/api/status did not answer — check: ssh $USER_AT_HOST 'cd $REMOTE_DIR && docker compose ${COMPOSE[*]} logs --tail 50'"
