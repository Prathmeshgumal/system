# Part 0.1 — Dev Environment: Design

- **Phase:** 0 — Foundations
- **Status:** ✅ done — experiments 1–2 recorded, 3–4 deferred (see [learnings.md](learnings.md))
- **Branch:** `part/0.1-dev-environment`
- **Plan:** skipped for this part (small scope; built directly from this design, tests first)

## Goal

One command starts everything; the browser shows a status page where React reports
"API healthy, Postgres healthy" — fetched from the Go API, which checks Postgres.
The same app also runs fully containerised, the way production will.

**Out of scope (later parts):** DB schema & migrations, structured logging, config
library, full graceful-shutdown sequence (0.2) · routing, TanStack Query, Tailwind (0.3).

## Decisions

| # | Decision | Chosen | Why | Note |
|---|---|---|---|---|
| A | Where Go & React run in dev | **Hybrid + full mode**: Go and Vite on the host, Postgres in Docker; `make up-full` runs everything in containers | Fast daily loop *and* learn Dockerfiles now; Phase 2 reuses the images | [Note 3](../../../Notes.md#3-multi-stage-docker-builds--base-images) |
| B | Command runner | **make** | Already installed, standard in Go repos | — |
| C | How React reaches the API | **Same origin via reverse proxy** (Vite in dev, Caddy in full/prod) | No CORS, simple cookies for auth in 1.1, same proxy grows into the LB | [Note 1](../../../Notes.md#1-same-origin-policy-cors--reverse-proxies) |
| D | Health checks | **`/livez` + `/readyz`** (startup probe later) | Avoid the DB-blip restart storm; ready for LBs and Kubernetes | [Note 2](../../../Notes.md#2-health-checks-liveness-vs-readiness) |
| E | Go runtime image | **`gcr.io/distroless/static:nonroot`** | Tiny, non-root, has CA certs + tzdata | [Note 3](../../../Notes.md#3-multi-stage-docker-builds--base-images) |

Smaller choices (made by Claude, overridable): `air` for Go live reload · `compose.yaml`
at repo root · Postgres 18 · ports 5173 (Vite), 8080 (API), 5432 (Postgres), 8000 (full mode).

## Section 1 — Repo layout & how it runs ✅ approved

```
system/
├── Makefile                 # every command lives here (`make help`)
├── compose.yaml             # postgres always; api + web only in the "full" profile
├── .env.example             # copy to .env; DB user/password/name, ports
├── backend/
│   ├── go.mod
│   ├── cmd/api/main.go      # starts the HTTP server
│   ├── internal/health/     # /livez, /readyz, /api/status (+ tests)
│   ├── Dockerfile           # golang → distroless, multi-stage
│   └── .air.toml            # live-reload config
├── web/
│   ├── (Vite + React + TS)
│   ├── vite.config.ts       # dev proxy: /api → :8080 (/ws added in Part 1.3)
│   ├── Dockerfile           # node (build) → caddy (serve), multi-stage
│   └── Caddyfile            # serve React, forward /api → api:8080 (/ws in Part 1.3)
└── knowledge-base/
```

```
DEV MODE (daily)                          FULL MODE (production-like check)
make dev                                  make up-full

 your machine                              docker network
 ├─ vite      :5173 ──proxy──┐             ├─ web (caddy) :8000 ─┐
 ├─ go (air)  :8080 ◄────────┘             ├─ api (distroless)  ◄┘  reaches DB at "postgres:5432"
 └─ docker: postgres :5432                 └─ postgres
 open http://localhost:5173               open http://localhost:8000
```

| Command | What it does |
|---|---|
| `make help` | List all commands |
| `make db` | Start only Postgres in Docker |
| `make api` | Run the Go API with live reload |
| `make web` | Run the Vite dev server |
| `make dev` | Postgres + API + web together in one terminal |
| `make up-full` | Build the real images and run everything in containers |
| `make down` | Stop everything (data kept) |
| `make clean` | Stop everything and delete the database volume |
| `make psql` | Open a SQL shell into the database |
| `make test` | Run backend + frontend tests |
| `make smoke` | End-to-end check of full mode (see Section 3) |

**Configuration & data**
- The Makefile loads `.env` and exports it, so `make api` gets the same values Compose uses.
- `DATABASE_URL` differs by mode — and that difference is the lesson of experiment 4:
  - dev: `postgres://…@localhost:5432/relay` (API on the host → Postgres's published port)
  - full: `postgres://…@postgres:5432/relay` (API container → Postgres by its Compose service name)
- Postgres data lives in a named volume (`pgdata`), so it survives `make down`;
  `make clean` removes it.
- `version` in `/api/status` defaults to `dev`; full-mode builds inject the git commit via `-ldflags`.

## Section 2 — What the code does ✅ approved

### Go backend — endpoints

| Endpoint | Audience | Response | Exposed by proxy? |
|---|---|---|---|
| `GET /livez` | Docker / LB / k8s | `200 {"status":"alive"}` | ❌ |
| `GET /readyz` | Docker / LB / k8s | `200 {"status":"ready"}` or `503 {"status":"not_ready"}` | ❌ |
| `GET /api/status` | React status page | `200`/`503` + `{"api":"ok","postgres":{"status":"ok","latency_ms":2},"version":"dev","uptime_s":42}` | ✅ |

`/readyz` and `/api/status` share one checker in `internal/health`, so they can never disagree.
Infra endpoints stay minimal and private; the public one carries the friendly details.

### Go backend — behaviour

1. **Startup:** read `PORT` and `DATABASE_URL` from env; create a `pgxpool` pool. The pool
   connects lazily, so **the API starts even if Postgres is down** — `/readyz` returns 503
   until the DB is reachable, then recovers by itself.
2. **Readiness check:** `Ping` (≈ `SELECT 1`) with a **1 s timeout**; latency measured.
3. **Server timeouts:** `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`
   set explicitly (Go's zero-value server has none → slowloris risk).
4. **Basic graceful shutdown:** on `SIGINT`/`SIGTERM` → `Server.Shutdown` (≤ 10 s for in-flight
   requests) → close pool → exit. Full sequence (fail readiness first, then drain) is Part 0.2.
5. **`api healthcheck` subcommand:** distroless has no `curl`, so the binary checks itself —
   calls `http://localhost:$PORT/livez`, exits 0/1. Used by the Compose healthcheck.
6. **Logging:** `log/slog` to stdout (structured setup in 0.2).

### React app

One page, **System Status**, polling `/api/status` every 5 s:

```
┌─ Relay — System Status ─────────────────┐
│  API        ● healthy                   │
│  Postgres   ● healthy   (2 ms)          │
│  Version    dev   ·   up 42s            │
│  Last check 3s ago                      │
└─────────────────────────────────────────┘
```

Three states, each reproducible by stopping a container:

| State | How it looks to React | How to trigger |
|---|---|---|
| All healthy | `200` with details | everything running |
| DB down | `503` with details (`postgres.status = "down"`) | `docker compose stop postgres` |
| API unreachable | `fetch` throws, or proxy returns `502` | stop the API |

Plain `fetch` + `useEffect`, minimal CSS — routing, TanStack Query and Tailwind are Part 0.3.

### Full mode

- Caddy serves the built React files and forwards `/api/*` → `api:8080` (`/ws` added in Part 1.3).
- Healthchecks: Postgres `pg_isready`; API `/api healthcheck`.
- API `depends_on` Postgres being **started**, not healthy — readiness handles the gap, and
  restarting Postgres while the app runs proves it.

## Section 3 — Testing & "done when" ✅ approved

### Automated tests (written first — TDD)

**Go — `internal/health` (unit)**
- The checker depends on a small `Pinger` interface, so tests use a fake DB:
  - ping succeeds → `ok` with measured latency
  - ping returns an error → `down`
  - ping hangs → `down` after ≈ 1 s (proves the timeout works)
- Handlers via `httptest`:
  - `/livez` → always `200`, **even when the DB is down**
  - `/readyz` → `200` / `503` matching the checker
  - `/api/status` → correct JSON shape and status code
- Tests against a real Postgres are set up in Part 0.2.

**React — status page (Vitest + React Testing Library, mocked `fetch`)**
- Renders all three states: healthy, DB down, API unreachable.

**Smoke test — `make smoke` (end to end, full mode)**
- Brings up full mode, then via Caddy on `:8000`:
  - `/api/status` → `200` with healthy JSON
  - `/livez` → does **not** return the API's JSON (Caddy serves the React page instead),
    proving infra endpoints aren't exposed through the proxy
- Tears everything down.

### Hands-on experiments (results → `learnings.md`)

1. **DB blip:** stop Postgres → page shows "DB down" within ~5 s, `/livez` stays `200`,
   API is not restarted → start Postgres → recovers on its own.
2. **API crash:** stop the API → page shows "API unreachable".
3. **Image sizes:** build the API image on `scratch`, `distroless/static` and `alpine`; compare.
4. **The localhost trap:** in full mode set `DATABASE_URL` host to `localhost` → watch it
   fail → explain why (each container has its own `localhost`).

### Done when

- [ ] Fresh checkout: `cp .env.example .env && make dev` → status page at `:5173` shows all healthy
- [ ] `make up-full` → all healthy at `:8000`; `docker compose ps` shows api + postgres **healthy**
- [ ] `make test` and `make smoke` pass
- [ ] All 4 experiments done and written up in `learnings.md`
- [ ] New concepts added to `Notes.md`; tracker shows 0.1 as ✅
