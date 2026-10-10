# Relay (working name)

A chat + social app built to learn system design hands-on: Go backend, React frontend,
deployed on AWS and grown from 100 users toward 1M.

- Plan: [knowledge-base/00-roadmap.md](knowledge-base/00-roadmap.md)
- What each version does: [knowledge-base/01-app-versions.md](knowledge-base/01-app-versions.md)
- Progress: [knowledge-base/phases/README.md](knowledge-base/phases/README.md) — Phases 1 and 2 (v0.2) are built
- Deploying to AWS: [deploy/README.md](deploy/README.md)

## Quick start

Needs: Docker, Go 1.27+, Node 24+, pnpm, make. Optional: [k6](https://grafana.com/docs/k6/latest/set-up/install-k6/)
for `make loadtest` (falls back to k6 in Docker).

```sh
cp .env.example .env     # then change the passwords, S3 secret and JWT_SECRET inside it
make dev                 # Postgres + storage + Go API (live reload) + React — open the URL Vite prints
make help                # every other command
```

Open the app, sign up, then sign up a second user in a private window and start a chat.
`make up-full` runs everything in containers at http://localhost:8000; `/status` shows health.
`make up-full-monitoring` adds Prometheus, Loki and Grafana (http://localhost:3000, user `admin`).

| Check | Command | Needs |
|---|---|---|
| Unit + integration tests | `make test` | `make db` running (tests create and drop their own databases) |
| Browser tests (two users chatting) | `make e2e` | first time: `cd web && pnpm exec playwright install chromium` |
| Full mode end to end | `make smoke` | Docker |
| 100-user load test (Part 1.4) | `make loadtest` | the API running (`make dev`) |
| Soak / breakpoint (Part 2.4) | `k6 run -e BASE_URL=… loadtest/soak.js` / `breakpoint.js` | a running stack — see [2.4 learnings](knowledge-base/phases/phase-2/2.4-observability/learnings.md) |
| Deploy | `make infra-apply`, then `make deploy` | AWS account, [deploy/README.md](deploy/README.md) |

## Layout

| Folder | What |
|---|---|
| `knowledge-base/` | Design docs, D2 diagrams, decision records |
| `backend/` | Go backend |
| `web/` | React + TypeScript app |
| `compose.yaml`, `Makefile` | Run everything (`make help`) |
| `deploy/` | Production compose, monitoring, Terraform, runbook |
| `scripts/` | Smoke test, deploy |
| `loadtest/` | k6 load tests |
