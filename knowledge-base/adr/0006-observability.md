# 0006 — Observability: Prometheus metrics, JSON logs in Loki, provisioned Grafana, on the same box

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 2 (Part 2.4)

## Context
Once the app is on the internet we need to know: is it up, is it fast, what's failing, and
how close is the box to its limits — without logging in and grepping. $0, and it must fit next to
the app on a 1 GiB instance.

## Options considered
1. **CloudWatch** — built in, but custom metrics and log ingestion cost money beyond small
   free allowances, and it ties the lessons to AWS.
2. **Hosted Grafana Cloud free tier** — no memory cost on our box, but sends our data out and
   has its own limits/accounts.
3. **Self-hosted Prometheus + Loki + Grafana (+ Alloy to ship logs)** in Docker — the de-facto
   open-source stack, the same locally and in production, ~350 MiB of RAM.

## Decision
Option 3, as a `monitoring` compose profile included by both full mode and production.

- **Metrics:** the API exposes Prometheus metrics on a **private** port (`METRICS_ADDR`, never
  routed by Caddy). RED per route (rate, errors, duration histogram), labelled by **mux pattern**
  (`/api/chats/{id}`) not raw path, plus WebSocket connections/events/drops, messages sent, DB pool,
  bcrypt slots, Go runtime and process stats.
- **Logs:** `LOG_FORMAT=json`, every request line has `request_id`, `route`, `status`,
  `duration_ms`, `user_id`; responses and error bodies carry `X-Request-ID`. Alloy reads container
  output from Docker and pushes it to Loki with a `service` label.
- **Dashboards as code:** data sources and the "Relay — overview" dashboard are provisioned from
  files in the repo; Grafana listens on 127.0.0.1 only (SSH tunnel in production).
- Retention: 7 days (Prometheus also capped at 1 GB). Memory limits measured, not guessed.

## Consequences
- ✅ One request ID ties a user's error to its log line; the dashboard answers "is it slow, where?".
- ✅ Same dashboards locally (`make up-full-monitoring`) and in production.
- ❌ Monitoring dies with the box it monitors, and competes with it for memory. Moving it to its
  own instance (or a hosted service) is a Phase 3 lesson.
- ❌ No alerting yet: dashboards only. Alerts (Grafana alerting → e-mail) come with SLOs in Phase 7,
  or earlier if something bites.
