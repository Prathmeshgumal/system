# Part 0.1 — Dev Environment: Design

- **Phase:** 0 — Foundations
- **Status:** 🟦 in design (brainstorming) — Section 1 approved, Section 2 in review, Section 3 pending
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

## Section 2 — What the code does ⏳ in review

*(Filled in once approved.)*

## Section 3 — Testing & "done when" ⬜

*(Filled in once approved.)*
