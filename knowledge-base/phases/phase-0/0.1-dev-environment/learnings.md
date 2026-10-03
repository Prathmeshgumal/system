# Part 0.1 — Dev Environment: Learnings

What we measured, what broke, and what you learned. The experiments below are **yours
to run** — the results sections are for your own words and numbers.

## What was built

| Piece | Where | Read this first |
|---|---|---|
| Health checker (timeout-bounded DB ping) | `backend/internal/health/checker.go` | the `Pinger` interface and `context.WithTimeout` |
| `/livez`, `/readyz`, `/api/status` | `backend/internal/health/handler.go` | why `livez` checks nothing |
| `api healthcheck` probe | `backend/internal/health/probe.go` | why distroless needs this |
| Server: timeouts, graceful shutdown | `backend/cmd/api/main.go` | `signal.NotifyContext`, `srv.Shutdown` |
| API image | `backend/Dockerfile` | the two `FROM` lines |
| Status page | `web/src/status/StatusPage.tsx`, `api.ts` | how 200 / 503 / failure become three states |
| Dev proxy | `web/vite.config.ts` | `server.proxy` |
| Full-mode proxy | `web/Caddyfile` | `handle /api/*` vs everything else |
| Orchestration | `compose.yaml`, `Makefile`, `scripts/smoke.sh` | profiles, healthchecks, `depends_on` |

Tests: 13 Go tests (`go test ./...` in `backend/`), 6 React tests (`pnpm test` in `web/`), `make smoke`.

## Already observed while building

- **API starts with no database at all:** `/livez` 200, `/readyz` 503, `/api/status` 503 with
  `postgres: down`. `pgxpool.New` doesn't connect until first use.
- **DB blip against real Postgres:** stopping Postgres flipped `/api/status` to 503 within
  one request while `/livez` stayed 200; starting it again recovered **without restarting the API**.
- **Image sizes:** `relay-api` = **20 MB** (distroless + static binary). The `golang:1.27`
  build image it came from is ~800 MB+. `relay-web` ≈ 89 MB — almost all of it is the
  `caddy:2-alpine` base; the React build itself is ~220 KB.
- **Port clash:** port 5173 was already used by another project on this machine, so Vite
  picked 5174 automatically. Check the URL Vite prints.

## Experiments to run

### 1. DB blip
1. `make dev`, open the status page.
2. In another terminal: `docker compose stop postgres`. Watch the page (≤ 5 s).
3. `curl -i localhost:8080/livez` and `curl -i localhost:8080/readyz`.
4. `docker compose start postgres`. Watch it recover. Did the API restart? (check its logs)

**Your results:** ✅ done 2026-10-03
- Page: **API healthy, Postgres down** within one poll after stopping Postgres.
- `/livez` → `200 {"status":"alive"}` · `/readyz` → `503 {"status":"not_ready"}` — exactly
  the split from Note 2: alive (don't restart) but not ready (don't send users).
- After `docker compose start postgres` the page went back to healthy **on its own**, and
  the API was not restarted.
- Lesson: the backend survives a database outage and recovers without anyone touching it,
  because liveness doesn't depend on the DB.

### 2. API crash
1. With `make dev` running, stop just the API (Ctrl+C stops both — instead run `make db`,
   `make web` and `make api` in three terminals, then Ctrl+C the API one).
2. What does the page show? What does the browser's Network tab show for `/api/status` — and who answered (Vite or Go)?

**Your results:** ✅ done 2026-10-03
- Ctrl+C on the API printed `shutting down` → `stopped cleanly`: **graceful shutdown** worked.
- Page: **API unreachable, Postgres unknown** (with the API gone, nobody can report on the DB).
- Network tab: `200 … 200` then **`502`** from the moment the API stopped. Every request went
  to `localhost:5173/api/status` — the browser never talks to `:8080` (the proxy, Note 1).
- Who answers when Go is dead? **Vite.** Through the proxy:
  `curl -i localhost:5173/api/status` → `502 Bad Gateway`, `Content-Type: text/plain`
  (not our JSON → React shows "unreachable"). Directly: `curl localhost:8080/api/status` →
  `Could not connect to server` — nobody is listening.
  - **502** = the proxy is up, the server behind it isn't (receptionist: "they're unavailable").
  - **Connection refused** = nothing at that address at all (the phone just rings out).
- Restarting the API (`make api`) → page went green **by itself** within one poll.
- Gotcha hit: the Vite port moved from 5174 (yesterday, 5173 was taken by another project)
  back to **5173** today. Always use the URL Vite prints.

### 3. Image sizes
Change the final `FROM` in `backend/Dockerfile`, rebuild with
`docker build -t relay-api:<name> backend`, and compare with `docker images relay-api`:
- `gcr.io/distroless/static:nonroot` (current)
- `scratch` — also remove the `USER` line; does `/api/status` still work? What would break with HTTPS calls?
- `alpine:3` — then `docker run --rm -it --entrypoint sh relay-api:alpine`. Can you do that with distroless?

Revert the Dockerfile afterwards. Bonus: `docker history relay-api:latest`.

**Your results:**

| Base | Size | Shell? | Notes |
|---|---|---|---|
| distroless/static | | | |
| scratch | | | |
| alpine | | | |

### 4. The localhost trap
1. In `compose.yaml`, change the api's `DATABASE_URL` host from `postgres` to `localhost`.
2. `make up-full`, open http://localhost:8000. What happens? Is the api container "healthy"? Why?
3. Explain in one paragraph why `localhost` inside a container isn't your machine.
4. Revert.

**Your results:**

## Questions to answer for yourself

- Why does `/readyz` exist if `/api/status` already reports the DB?
- What would happen in production if `/livez` pinged the database?
- Why does `compose.yaml` publish Postgres's port but not the API's?
- In `Dockerfile`, what happens to build time if you move `COPY . .` above `go mod download`? Try it.
