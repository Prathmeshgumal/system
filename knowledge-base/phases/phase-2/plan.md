# Phase 2 — Live on the internet (v0.2): Build Plan

Spec: [design.md](design.md). Test first, one commit per task.

## Step 1 — Part 2.1 Richer chat

| # | Task | Where | Tests |
|---|---|---|---|
| 1.1 | Migration `0003`: `message_reactions`, `messages.reply_to_id` | `backend/migrations/` | — |
| 1.2 | Store: reactions add/remove/list-for-messages; send with `reply_to_id`; quotes for a page | `internal/chat/store.go` | integration |
| 1.3 | Service + HTTP: reaction endpoints, reply validation, `MessageView.reactions/reply_to` | `internal/chat/` | API tests |
| 1.4 | `chat.read` to all members with `user_id` | `internal/chat/service.go` | WS test |
| 1.5 | WebSocket reads client frames; `typing` relay with membership check + rate limit | `internal/realtime/` | WS tests |
| 1.6 | Web: reactions bar + picker, reply (hover action, composer quote, quote in bubble) | `web/src/chat/` | unit |
| 1.7 | Web: typing send/throttle + indicator; receipts ✓/✓✓ and "Seen by N" | `web/src/chat/`, `realtime/` | unit |
| 1.8 | Web: notifications + title badge; theme toggle (no flash) | `web/src/` | unit |
| 1.9 | E2E: typing, receipts, reactions, replies | `web/e2e/` | Playwright |

## Step 2 — Part 2.2 Profile pictures

| # | Task | Where | Tests |
|---|---|---|---|
| 2.1 | Config + `storage` package (minio-go): presign POST, stat, get, delete, ensure bucket | `internal/config`, `internal/storage/` | integration vs RustFS |
| 2.2 | Migration `0004`: `users.avatar_key`; `Profile.avatar_url` | `migrations/`, `internal/users/` | — |
| 2.3 | Avatar endpoints (upload URL, confirm, delete) + `GET /media/{key...}` | `internal/users/` (or `media/`) | unit (fake storage) + API |
| 2.4 | RustFS in compose; `/storage` proxy in Vite and Caddy; `/media` proxy | `compose.yaml`, `web/` | smoke |
| 2.5 | Web: picture picker, canvas resize, upload, `Avatar` shows pictures | `web/src/profile/`, `ui/` | unit + e2e |

## Step 3 — Part 2.4 Observability (before 2.3: the deploy ships it)

| # | Task | Where | Tests |
|---|---|---|---|
| 3.1 | JSON logs + request IDs (header, error bodies, every log line) | `internal/httpx` | unit |
| 3.2 | Metrics: registry, HTTP middleware with route pattern, WS/chat/pool/hash metrics, private listener | `internal/metrics/`, others | unit |
| 3.3 | Monitoring compose profile: Prometheus, Loki, Alloy, Grafana + provisioned dashboard | `deploy/monitoring/` | run it, query it |

## Step 4 — Part 2.3 AWS deploy

| # | Task | Where |
|---|---|---|
| 4.1 | Production compose + `Caddyfile.prod` + `.env.prod.example`; run locally with HTTPS | `deploy/`, `web/` |
| 4.2 | `scripts/deploy.sh` + `make deploy`; test against this machine over SSH | `scripts/`, `Makefile` |
| 4.3 | Terraform: network, SG, key, EC2 (standard credits, IMDSv2, user data), EIP, S3 + CORS + IAM, budget | `deploy/terraform/` |
| 4.4 | Runbook: console walkthrough, guard-rails, deploy, rollback, destroy, Grafana tunnel | `deploy/README.md`, learnings |

## Step 5 — Measure (Part 2.4 continued)

| # | Task | Where |
|---|---|---|
| 5.1 | `loadtest/soak.js` (100 users × 30 min), `loadtest/breakpoint.js` (ramp until SLO breaks) | `loadtest/` |
| 5.2 | Run both against the prod stack capped at t3.micro size (2 CPU burst, then 0.2 CPU baseline), with the dashboard open | — |
| 5.3 | Write up: `concepts/capacity-one-micro-instance.md`, 2.4 learnings | knowledge-base |

## Step 6 — Docs

ADRs 0004 (object storage & uploads), 0005 (single-instance deploy), 0006 (observability);
Notes 8 (temporary vs stored events & write amplification), 9 (metrics: RED/USE, cardinality),
10 (object storage & presigned URLs), 11 (networking for one server: VPC, SG, DNS, TLS);
phase README, tracker, per-part learnings, README quick start.
