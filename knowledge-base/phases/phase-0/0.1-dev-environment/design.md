# Part 0.1 — Dev Environment: Design

- **Phase:** 0 — Foundations
- **Status:** 🟦 in design (brainstorming) — Sections 1–2 approved, Section 3 in review
- **Branch:** to be created from `main` when the design is approved

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
├── .env.example             # copy to .env; DB password, ports
├── backend/
│   ├── go.mod
│   ├── cmd/api/main.go      # starts the HTTP server
│   ├── internal/health/     # /livez, /readyz, /api/status (+ tests)
│   ├── Dockerfile           # golang → distroless, multi-stage
│   └── .air.toml            # live-reload config
├── web/
│   ├── (Vite + React + TS)
│   ├── vite.config.ts       # dev proxy: /api, /ws → :8080
│   ├── Dockerfile           # node (build) → caddy (serve), multi-stage
│   └── Caddyfile            # serve React, forward /api + /ws → api:8080
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
| `make down` | Stop everything |
| `make psql` | Open a SQL shell into the database |
| `make test` | Run backend + frontend tests |

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

## Section 3 — Testing & "done when" ⏳ in review

*(Filled in once approved.)*
