# Phase 2 — Live on the internet (v0.2): Design

- **Phase:** 2 — Ship it
- **Status:** 🟨 building (see [plan.md](plan.md))
- **Branch:** `claude/dreamy-franklin-1jaomb` (on top of Phase 1)
- **Plan:** [plan.md](plan.md)

## Goal

Friends anywhere can use Relay at a real HTTPS URL, with the small things that make chat
feel alive — typing, read receipts, reactions, replies, notifications, profile pictures,
dark mode — and you can *see* how the server is doing: a Grafana dashboard, searchable
logs, and a measured breaking point for one micro instance.

**v0.2 done when** ([01-app-versions](../../01-app-versions.md#v02--live-on-the-internet-end-of-phase-2)):
100 simulated users on the server for 30 min with p95 delivery < 300 ms; the breaking
point and bottleneck written up; the whole setup rebuilt by one command.

**Out of scope:** multiple servers / load balancer (3.1), Redis (3.x), email (3.3), CI/CD
(needs GitHub Actions — later), CDN (4.3).

### What can and can't be verified before you run it on AWS

This was built in a sandbox without AWS credentials (and creating AWS resources is your
call, not mine). So:

| Verified here | Left for you |
|---|---|
| All app features, unit/integration/e2e tests | `terraform apply` on your account |
| The production compose stack + HTTPS Caddy config, run locally | The real Let's Encrypt certificate |
| `make deploy` end to end, against this machine over SSH | `make deploy` to the EC2 instance |
| Observability stack, dashboard, logs | Watching it on the real box |
| Soak + breakpoint load tests against a **t3.micro-sized** local stack (2 CPUs → 0.2 CPU baseline) | The same scripts against AWS — numbers to compare |
| `terraform fmt` | `terraform validate/plan` (provider downloads are blocked here) |

---

## Decisions

Chosen by Claude while you were away — all overridable.

| # | Decision | Chosen | Why |
|---|---|---|---|
| A | Typing indicators | **Client → server WebSocket frame** `{"type":"typing","chat_id":…}`, relayed to the other members, **never stored**. Client sends at most every 3 s while typing; the indicator expires after 6 s | The first event that's *temporary*: losing one costs nothing, storing them would cost a write per keystroke. First client → server frame (ADR 0003 anticipated it) |
| B | Read receipts | **Reuse `last_read_seq`**: `chat.read {chat_id, user_id, seq}` now goes to *every member*, not just my tabs. 1:1: ✓ sent / ✓✓ seen. Groups: "Seen by N" under my latest message | No new table. The cost is write amplification: one read = one event per member (Note 8) |
| C | Reactions | **`message_reactions(message_id, user_id, emoji)`**, fixed set 👍 ❤️ 😂 😮 😢 🙏, `PUT`/`DELETE /api/messages/{id}/reactions/{emoji}` (idempotent), broadcast as `message.updated` | Fixed set = trivial validation, tiny storage. Loaded per history page in one query (N+1 again) |
| D | Replies | **`messages.reply_to_id`** (same chat only); the quote is read live with the page (one batch query), not copied into the reply | An edit or delete of the original shows in the quote |
| E | Notifications & title badge | **Notification API** while the tab is hidden; `(3) Relay` in the title; permission asked from a button | Browsers ignore permission prompts not triggered by a click |
| F | Dark mode | **System / light / dark** toggle, `.dark` class on `<html>`, saved in `localStorage`, applied by an inline script before React loads | No white flash on load |
| G | Object storage | **S3 API via `minio-go`**: AWS S3 in production, **RustFS** locally (MinIO stopped publishing Docker images; RustFS speaks the same API) | Same code against both; the client library works with any S3 |
| H | Uploads | **Presigned POST**, straight from the browser to storage, then a confirm call. The policy enforces **≤ 1 MB and `image/jpeg|png|webp`**; the browser first shrinks the picture to 256 × 256 | Files never pass through the API; size limits enforced by storage itself — a presigned *PUT* can't limit size (ADR 0004) |
| I | Serving pictures | **`GET /media/{key}` through the API**, keys are random and immutable → `Cache-Control: immutable, 1 year` | Private bucket, same origin, no CORS for reads; a CDN slots in front later (4.3) |
| J | Hosting | **One EC2 `t3.micro`** (free tier), Docker Compose: Caddy + API + Postgres (+ monitoring). **`cpu_credits = "standard"`** | `t3` defaults to *unlimited* credits, which bills for sustained CPU — the classic surprise charge (ADR 0005) |
| K | HTTPS & name | **Caddy automatic HTTPS** (Let's Encrypt); free hostname **`<ip>.sslip.io`** by default, any domain via `DOMAIN` | $0 and a real certificate on day one |
| L | Infrastructure as code | **Terraform**: VPC, subnet, internet gateway, security group, key pair, EC2 + Elastic IP, S3 bucket + IAM role, **zero-spend budget alert**. Console walkthrough first in [2.3 learnings](2.3-aws-deploy/learnings.md) | One command rebuilds it; `terraform destroy` makes "$0" easy to keep |
| M | Deploy | **`make deploy`**: build images locally → `docker save \| ssh … docker load` → copy compose file + secrets → `docker compose up -d` → smoke check. No registry | No registry account, nothing new to secure; deploys are one command (ADR 0005) |
| N | Secrets | `deploy/.env.prod` on your laptop only (git-ignored), copied with `chmod 600`; S3 access via the **instance's IAM role**, no keys at all | The best secret is the one that doesn't exist |
| O | Metrics | **Prometheus** scraping `/metrics` on a private port (9090, never through Caddy): RED per route, WebSocket gauges, DB pool, bcrypt slots, Go runtime. Route label = the **mux pattern** (`/api/chats/{id}`), not the raw path | Bounded label cardinality — raw paths would make one time series per chat id (Note 9) |
| P | Logs | **JSON logs with request IDs** (`LOG_FORMAT=json`), shipped by **Grafana Alloy** to **Loki** | Searchable in Grafana next to the metrics; Promtail is end-of-life |
| Q | Dashboards | **Grafana**, data sources and one dashboard provisioned from files in the repo | Dashboards as code: rebuilt with the box |
| R | Where monitoring runs | **Same box**, a `monitoring` compose profile, sized to fit 1 GiB with 2 GiB swap | $0; moving it off-box is a Phase 3 lesson ("don't let the patient monitor itself") |
| S | Load tests | **Soak**: 100 users × 30 min. **Breakpoint**: ramp users until delivery p95 > 300 ms or errors appear. Locally against a stack capped to t3.micro size, then on AWS by you | Find the limit *and* what breaks first |

---

## Section 1 — Back-of-envelope: what does a t3.micro give us?

| Resource | t3.micro | What uses it |
|---|---|---|
| CPU | 2 vCPU **burst**, **20 % baseline** (0.2 vCPU sustained), 12 credits/hour | bcrypt, JSON, TLS |
| Memory | 1 GiB | Postgres ~100 MB, API ~30 MB, Caddy ~30 MB, Prometheus ~150 MB, Grafana ~100 MB, Loki + Alloy ~150 MB → ~560 MB, + 2 GiB swap |
| Disk | 30 GB gp3 free tier | Postgres, Prometheus (7-day retention), Loki (7 days) |
| Network | "up to 5 Gbit/s" | negligible: 100 users ≈ 60 kB/s (Part 1.4) |

**CPU credits are the trap.** Part 1.4 used ~37 % of 4 cores while 100 users chatted *and* 150
bcrypt hashes ran; chatting alone used ~1.5 CPU-seconds per minute (≈ 2.5 % of one core).
Prediction: **100 chatting users fit comfortably inside the 20 % baseline; logins and
signups burn credits; sustained load above ~0.2 vCPU throttles hard once credits run out.**
The breakpoint test is run twice: at full burst (2 CPUs) and at baseline (0.2 CPU).

---

## Section 2 — Part 2.1: Richer chat

### Typing (temporary event)

```
client: keydown in composer → at most once per 3 s → WS frame {"type":"typing","chat_id":7}
server: is the sender a member of chat 7? (one indexed lookup) → publish to the OTHER members
        {"type":"typing","chat_id":7,"user_id":3}            (rate limit: drop > 2/s per connection)
client: show "Alice is typing…" for 6 s after the last event; a message from Alice clears it
```

Nothing touches the database except the membership check. If an event is lost, the indicator
just doesn't show — acceptable for a temporary event, unacceptable for a message.

### Read receipts (stored event, wider fan-out)

`POST /api/chats/{id}/read` is unchanged; its event now carries `user_id` and goes to every
member. Clients keep each member's `last_read_seq` in the chat cache:

- 1:1: my message shows **✓** (sent) or **✓✓** (the other person's `last_read_seq ≥ seq`)
- group: under my latest message, **"Seen by N"** (members other than me with `last_read_seq ≥ seq`)

**Write amplification:** in a group of 50 where everyone reads every message, one message
causes 50 read events × 50 recipients = 2,500 deliveries. The client already sends at most one
read per newest seq; that's the lever if it ever matters (Note 8).

### Reactions

```sql
message_reactions(message_id → messages ON DELETE CASCADE, user_id → users, emoji text,
                  created_at, PRIMARY KEY (message_id, user_id, emoji))
```
`PUT /api/messages/{id}/reactions/{emoji}` adds (200 even if already there), `DELETE` removes.
Members only; emoji must be in the fixed set. Messages carry
`"reactions": [{"emoji":"👍","user_ids":[3,9]}]`; the change is broadcast as `message.updated`.

### Replies

`messages.reply_to_id bigint NULL REFERENCES messages(id)`; `POST …/messages` accepts
`reply_to_id`, which must be a message in the same chat. Messages carry
`"reply_to": {"id","seq","sender","body","deleted"}` (body cut to 200 characters), loaded for
the whole page in one query.

### Notifications, title badge, dark mode

All client-side. Notifications only for messages from others, only while the tab is hidden,
grouped per chat (`tag`), clicking one focuses the tab and opens the chat.

### History query cost

A page now needs: messages (1 query) + senders (1) + reactions (1) + replied-to messages (1)
= **4 queries per page, whatever the page size** — the batch pattern from Part 1.2.

---

## Section 3 — Part 2.2: Profile pictures (object storage)

```
browser                         API                               storage (S3 / RustFS)
  │ 1 pick photo; shrink to 256×256 JPEG in a <canvas>
  │ 2 POST /api/me/avatar/upload {content_type} ──►  key = avatars/<user>/<random>.jpg
  │                                ◄── {url, fields}   policy: this key, this type, ≤ 1 MB, 5 min
  │ 3 POST url (multipart: fields + file) ───────────────────────────────────────────► stores it
  │ 4 PUT /api/me/avatar {key} ──►  key is mine? object exists, ≤ 1 MB, image/*?
  │                                 users.avatar_key = key; delete the old object
  │                                ◄── user {avatar_url: "/media/avatars/3/k2j….jpg"}
  │ 5 <img src="/media/…"> ──────►  GET from storage, stream back with
                                    Cache-Control: public, max-age=31536000, immutable
```

- **Why presigned POST, not PUT:** a POST policy can say `content-length-range 1–1048576` and
  `Content-Type = image/jpeg`; storage rejects anything else. A presigned PUT URL can't limit size.
- **Why a confirm step:** the API never trusts "I uploaded it" — it checks the object exists and
  matches, and only then points the user at it. An upload that's never confirmed stays in the
  bucket as an orphan; a cleanup job comes with the other background work in Phase 4.
- **Browser → storage path:** locally the browser posts to `/storage/…`, which Vite / Caddy
  forward to RustFS (same origin). In AWS it posts straight to the bucket's HTTPS endpoint;
  the bucket's CORS rule allows only our origin.
- `DELETE /api/me/avatar` removes it. Profiles everywhere gain `avatar_url` (null = initials).

Settings: `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`, `S3_ACCESS_KEY` / `S3_SECRET_KEY` (empty in
AWS: the instance role is used), `S3_USE_SSL`, `S3_PUBLIC_URL` (what the browser posts to),
`S3_CREATE_BUCKET` (dev only).

---

## Section 4 — Part 2.3: First AWS deploy

```
                    Internet
                       │  80/443 (anyone)        22 (your IP only)
             ┌─────────▼──────────────────────────────────────────┐
  VPC 10.0.0.0/16      │ public subnet 10.0.1.0/24                 │
             │   ┌─────┴─────────────── EC2 t3.micro (EIP) ─────┐ │      S3 bucket (private)
             │   │ Caddy :443 ──► /api,/ws,/media ─► api :8080 ─┼─┼────► avatars/…
             │   │   └─► React files        api ─► postgres     │ │  (IAM role, no keys)
             │   │ monitoring profile: prometheus, grafana,     │ │
             │   │ loki, alloy (Grafana via SSH tunnel only)    │ │
             │   └──────────────────────────────────────────────┘ │
             └────────────────────────────────────────────────────┘
```

| File | What |
|---|---|
| `deploy/terraform/*.tf` | network, security group, key pair, instance (+ user data: Docker, swap), EIP, S3 + CORS, IAM role, budget |
| `deploy/compose.prod.yaml` | caddy (web image + prod Caddyfile), api, postgres, monitoring profile |
| `web/Caddyfile.prod` | `{$DOMAIN}` → automatic HTTPS; same routes as full mode; security headers |
| `deploy/.env.prod.example` | every production setting; copy to `deploy/.env.prod` (git-ignored) |
| `scripts/deploy.sh` (`make deploy`) | build → ship → start → smoke check |
| `deploy/README.md` | runbook: first-time setup, deploy, rollback, destroy, SSH tunnel to Grafana |

**Guard-rails (in Terraform where possible):** zero-spend budget alert to your email; `standard`
CPU credits; gp3 20 GB (free tier is 30); IMDSv2 required; SSH only from your IP; no NAT gateway,
no load balancer; everything tagged `project = relay` so you can find it; `terraform destroy`
tears it all down. Manual, once: root MFA, an IAM admin user, region choice.

**Rollback:** every deploy tags images with the git SHA; `make deploy SHA=<older>` re-ships an
older build (images are kept locally).

---

## Section 5 — Part 2.4: Observability

### Metrics (`/metrics` on `METRICS_ADDR`, private)

| Metric | Type | Labels | Answers |
|---|---|---|---|
| `http_requests_total` | counter | `method`, `route`, `code` | rate, errors (RED) |
| `http_request_duration_seconds` | histogram | `method`, `route` | latency p50/p95/p99 |
| `ws_connections` | gauge | — | who's online |
| `ws_events_sent_total`, `ws_clients_dropped_total` | counter | `type` / `reason` | fan-out volume, slow clients |
| `chat_messages_sent_total` | counter | — | messages/s |
| `db_pool_connections` | gauge | `state` = in_use/idle/max | pool saturation (USE) |
| `db_pool_acquire_wait_seconds_total`, `db_pool_empty_acquire_total` | counter | — | waiting for a connection |
| `auth_hash_slots_in_use`, `auth_hash_queue_wait_seconds` | gauge / histogram | — | bcrypt bulkhead (1.4) |
| Go runtime + process (CPU, memory, goroutines, fds) | built in | — | the box itself |

### Logs

`LOG_FORMAT=json` → one JSON object per line with `request_id`, `method`, `route`, `status`,
`duration_ms`, `user_id` when known. Every response carries `X-Request-ID`; error bodies too,
so "it broke" + an ID finds the exact log line.

### Dashboard (provisioned): "Relay — overview"

Row 1: online users · messages/s · requests/s · 5xx rate · p95 latency.
Row 2: latency by route (p95) · requests by route · status codes.
Row 3: DB pool (in use / idle / max, waits) · bcrypt slots + queue wait · goroutines · memory · CPU.
Row 4: error logs (Loki: `{service="api"} | json | level="ERROR"`).

---

## Section 6 — Testing & done when

### Automated

- **Go:** typing relay + membership + rate limit (WS test client); reactions (idempotent add,
  remove, non-member 404, unknown emoji 400); replies (same chat only, quote reflects edit/delete);
  read events reach all members; avatar flow against a fake storage (unit) and against RustFS
  (integration, skipped without `TEST_S3_ENDPOINT`); `/media` cache headers; metrics route labels
  use the pattern; request IDs in responses and logs.
- **React:** typing indicator expiry, receipts ✓/✓✓ and "Seen by", reaction toggling,
  reply quote, title badge, theme toggle.
- **E2E (Playwright):** typing shows and clears; ✓✓ appears when the other reads; react and
  reply live; upload a profile picture and see it in the other browser.
- **Deploy:** `make deploy` against this machine over SSH, then `make smoke` against the
  deployed URL (HTTPS with Caddy's local CA).

### Done when

- [ ] All of the above pass
- [ ] Production stack runs locally with HTTPS; `make deploy` works over SSH
- [ ] Dashboard shows live data during a load test; error logs searchable in Grafana
- [ ] Soak: 100 users × 30 min on a t3.micro-sized stack, delivery p95 < 300 ms
- [ ] Breakpoint found at burst and at baseline CPU, bottleneck written up in `concepts/`
- [ ] ADRs 0004–0006, Notes 8–11, learnings per part, tracker updated
- [ ] **Yours:** `terraform apply`, `make deploy` to AWS, the same load tests there
