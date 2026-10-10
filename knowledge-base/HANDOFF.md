# Handoff — after Phases 1 and 2

Written 2026-10-10, right after [PR #1](https://github.com/Prathmeshgumal/system/pull/1)
merged Phases 1 and 2 into `main`. Read this before starting the next session.

## Where things stand

| Part | State |
|---|---|
| Phase 1 (1.1–1.4) | ✅ built, tested, written up |
| 2.1 Richer chat | ✅ built, tested, written up |
| 2.2 Media | ✅ built, tested, written up |
| 2.3 AWS deploy | 🟨 built and tested **locally only**; `terraform apply` never run |
| 2.4 Observability | 🟨 stack built and verified; **measurements unfinished**, two docs missing |
| 0.2 Backend skeleton | ⚠️ separate branch, conflicts with `main` — decision needed |

What "tested" means here: backend `go test -race` (Postgres + RustFS), 54 web unit tests,
7 Playwright e2e tests in dev mode, full mode and against the production stack deployed with
`make deploy` over SSH to localhost (rollback included).

## First things to do on your machine

1. `cp .env.example .env` again (or copy over the new `S3_*`, `LOG_FORMAT`, `METRICS_ADDR`,
   `GRAFANA_ADMIN_PASSWORD` lines). Without `S3_*`, avatars return 503 `storage_off`.
2. `make db` (now Postgres **and** RustFS), `make dev`, `make test`, `make e2e`.
3. `make up-full` then `make smoke` — **this is the first real test of the Dockerfiles** (see
   "Not verified" below). If the image build fails, `git revert cf17ee6`.
4. `make up-full-monitoring` → Grafana on http://localhost:3000, dashboard "Relay overview".

## Open work, in order

### 1. Fix the soak test's token expiry (small, do first)

The 30-minute soak (100 users, t3.micro-sized stack) **failed its thresholds**, and the cause is
the test script, not the server:

- `loadtest/lib.js` `session()` logs in once and reuses the access token. Access tokens live
  15 minutes (`AccessTTL` in `backend/internal/config/config.go`), so from minute 15 on every
  send and read got **401**: 9,535 of 29,194 requests (sends 48 % ok, reads 97 % ok — reads
  are fewer after the first 401 because nothing new arrives to mark read).
- Fix: in `session()`, call `POST /api/auth/refresh` (k6's cookie jar should already hold
  the `relay_refresh` cookie from login) every ~10 minutes and swap the token. Then re-run the soak.

What the soak *did* show, for the first 15 minutes and the WebSocket side throughout:

| Measure | Result |
|---|---|
| Delivery p95 (send → other members' sockets) | **9 ms** (target < 300 ms) ✅ |
| WebSocket errors | 0 ✅ |
| CPU, whole stack, steady state | avg **0.145 cores** (peak 1.07 during signup bcrypt) |
| Memory, whole stack incl. monitoring | **514–595 MiB**, flat after warm-up (no leak seen) |

### 2. Small bug: rejected requests are labelled `route="unmatched"`

The 401s above were logged with `"route":"unmatched"` on real routes like
`/api/chats/26/messages`. `httpx.Route` reads `r.Pattern` after the mux runs; when the auth
wrapper rejects before the inner mux matches, there is no pattern. Effect: per-route error
rates in Grafana miss every 401. Fix idea: wrap the outer mux with `Route` too, or set the route
in the auth middleware before rejecting. Add a test in `internal/httpx/observe_test.go`.

### 3. Breakpoint tests (Part 2.4's real goal)

Not run yet. `loadtest/breakpoint.js` is ready: it steps up users until delivery p95 or errors
blow up. Run it twice on a t3.micro-sized stack:

- **Burst** (2 vCPU): `docker update --cpus 2 --memory 1g` on each container, `GOMAXPROCS=2`.
- **Baseline** (t3.micro's sustained 20 % of 2 vCPU = 0.2 CPU): `--cpus 0.2`.
- Server needs `BCRYPT_COST=4` for this test (setup signs up thousands of users).
- Example: `k6 run -e BASE_URL=https://… -e MAX_USERS=2500 -e STEP=250 -e RAMP_MIN=15 loadtest/breakpoint.js`
  (baseline: `MAX_USERS=600 STEP=50`).

**Suspected first bottleneck: the DB pool.** It is still pgx's default (max 4 connections on
2 CPUs). In the short soak "in use" hit 4 and pool waits appeared on the dashboard. The 0.2
branch already has `DB_MAX_CONNS`; porting that (see item 5) fixes this too.

### 4. Write the two missing docs

Both are already linked (from `phases/README.md`, `phase-2/README.md`, `phase-2/plan.md`), so
those links are broken until they exist:

- `knowledge-base/phases/phase-2/2.4-observability/learnings.md`
- `knowledge-base/concepts/capacity-one-micro-instance.md` — the breakpoint numbers, what
  saturated first, and the fix; follow the concept-note format in `knowledge-base/README.md`.

### 5. Decide what to do with `part/0.2-backend-skeleton`

0.2 and Phases 1–2 built the same foundations twice (config, logging, migrations, DB access,
test database). Merging 0.2 into `main` now conflicts in 9 files. Options and a proposed port
path are in [phases/merging-0.2.md](phases/merging-0.2.md). Recommended: keep 0.2's platform
and port Phases 1–2 onto it, one commit per step, using the existing API, e2e and load tests as
the safety net.

### 6. The real AWS run (yours — needs your AWS account)

`deploy/README.md` is the runbook: `make infra-init`, `make infra-plan`, `make infra-apply`,
`make infra-env` → `deploy/.env.prod`, then `make deploy`. Then repeat items 1 and 3 against
the real instance with `BASE_URL=https://<ip>.sslip.io`.

## Not verified

- **Dockerfiles** (`cf17ee6`, cross-compile with `--platform=$BUILDPLATFORM`): never built —
  Docker Hub rate-limited pulls in the build environment.
- **Terraform**: could not be installed in the build environment; only formatting and
  cross-references were checked. No `plan`, no `apply`.
- **Let's Encrypt**: HTTPS was only tested with Caddy's local CA on `localhost`.

## Gotchas worth remembering

- MinIO no longer publishes Docker images; local S3 is **RustFS** (`rustfs/rustfs:1.0.1`).
- The production compose project is `relay-prod`, so it never reuses the dev `relay_pgdata`
  volume (different password → "password authentication failed").
- `API_MAX_OPEN_FILES` exists because some Docker hosts refuse `nofile=65536`.
- Grafana needs ~230 MiB idle; its limit is 320m (192m got it OOM-killed).
- Vite's proxy must keep `changeOrigin: false`, or the Origin check logs users out on reload.
