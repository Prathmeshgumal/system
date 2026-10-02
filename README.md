# Relay (working name)

A chat + social app built to learn system design hands-on: Go backend, React frontend,
deployed on AWS and grown from 100 users toward 1M.

- Plan: [knowledge-base/00-roadmap.md](knowledge-base/00-roadmap.md)
- What each version does: [knowledge-base/01-app-versions.md](knowledge-base/01-app-versions.md)

## Quick start

Needs: Docker, Go 1.27+, Node 24+, pnpm, make.

```sh
cp .env.example .env     # then change the password in both places inside it
make dev                 # Postgres + Go API (live reload) + React — open the URL Vite prints
make help                # every other command
```

`make up-full` runs everything in containers at http://localhost:8000.

## Layout

| Folder | What |
|---|---|
| `knowledge-base/` | Design docs, D2 diagrams, decision records |
| `backend/` | Go backend |
| `web/` | React + TypeScript app |
| `compose.yaml`, `Makefile` | Run everything (`make help`) |
| `scripts/` | Helper scripts (smoke test) |
| `loadtest/` | k6 load tests |
