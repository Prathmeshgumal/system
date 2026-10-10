# Notes — Concepts to Revisit

Important concepts that come up while we design and build. Each note is short enough to
re-read in 5 minutes, with pointers for going deeper in your free time.

Format per note: **What it is → Why it matters → What we chose → In our code → Gotchas → Go deeper → Interview angle.**

## Index

| # | Concept | Came up in |
|---|---|---|
| 1 | [Same-origin policy, CORS & reverse proxies](#1-same-origin-policy-cors--reverse-proxies) | Part 0.1, decision C |
| 2 | [Health checks: liveness vs readiness](#2-health-checks-liveness-vs-readiness) | Part 0.1, decision D |
| 3 | [Multi-stage Docker builds & base images](#3-multi-stage-docker-builds--base-images) | Part 0.1, decision E |
| 4 | [Sessions, JWTs & where tokens live](#4-sessions-jwts--where-tokens-live) | Phase 1, decisions B–C |
| 5 | [HTTP vs polling vs WebSockets](#5-http-vs-polling-vs-websockets) | Phase 1, decisions H–J |
| 6 | [Ordering & cursor pagination](#6-ordering--cursor-pagination) | Phase 1, decisions E–G |
| 7 | [Password hashing & CPU-heavy work](#7-password-hashing--cpu-heavy-work) | Phase 1, decision A + Part 1.4 |
| 8 | [Temporary vs stored events, and write amplification](#8-temporary-vs-stored-events-and-write-amplification) | Part 2.1, decisions A–B |
| 9 | [Metrics: RED, USE and label cardinality](#9-metrics-red-use-and-label-cardinality) | Part 2.4, decisions O–Q |
| 10 | [Object storage & presigned uploads](#10-object-storage--presigned-uploads) | Part 2.2, decisions G–I |
| 11 | [One server on the internet: VPC, firewall, IAM, TLS](#11-one-server-on-the-internet-vpc-firewall-iam-tls) | Part 2.3, decisions J–N |

---

## 1. Same-origin policy, CORS & reverse proxies

*Came up in Part 0.1 — how the React app reaches the Go API.*

![same origin vs cors](diagrams/notes/01-same-origin-proxy.svg)

### What it is

**Origin** = scheme + host + port. All three must match for two URLs to be the same origin:

| URL A | URL B | Same origin? |
|---|---|---|
| `http://localhost:5173` | `http://localhost:8080` | ❌ port differs |
| `https://relay.com` | `http://relay.com` | ❌ scheme differs |
| `https://app.relay.com` | `https://api.relay.com` | ❌ host differs |
| `https://relay.com/a` | `https://relay.com/b` | ✅ path doesn't matter |

**Same-origin policy (SOP):** the browser's core security rule. JavaScript on one origin
can't *read* responses from another origin. Without it, any website you visit could
call `fetch("https://yourbank.com/account")` — with your bank cookies attached — and read your data.

Subtle point: SOP mostly blocks **reading the response**, not **sending the request**.
A simple cross-origin POST still reaches the server; JS just can't see the reply.
That's why CSRF attacks exist (Part 1.1).

**CORS (Cross-Origin Resource Sharing):** the server's way of saying "I allow these
other origins to read my responses", using response headers:

```
Access-Control-Allow-Origin: https://app.relay.com
Access-Control-Allow-Credentials: true        # allow cookies
Access-Control-Allow-Methods: GET, POST, PATCH, DELETE
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Max-Age: 600                   # cache the preflight for 10 min
```

**Preflight:** for "non-simple" requests (JSON body, `Authorization` header, `PATCH`/`DELETE`…)
the browser first sends an `OPTIONS` request asking permission, and only sends the real
request if the answer is yes. That's an extra round trip (unless cached via `Max-Age`).

**Reverse proxy:** a server that sits in front of your app servers and forwards requests
to them. The client only sees the proxy. (A *forward* proxy is the opposite: it sits in
front of *clients*, like a corporate proxy.) Reverse proxies also do TLS, compression,
caching, load balancing — Nginx, Caddy, HAProxy, Envoy, AWS ALB are all reverse proxies.

### Why it matters for us

- The proxy makes React and the API **one origin**, so SOP never gets in the way —
  the proxy → Go hop is server-to-server, and browser rules don't apply there.
- Cookies stay simple: our refresh-token cookie (Part 1.1) is first-party.
- The same proxy will later become our **load balancer** (Phase 3) — one concept, growing.

### What we chose

**Same origin via reverse proxy** (Part 0.1, decision C):

| Mode | Proxy | Routes |
|---|---|---|
| dev | Vite dev server | `/` → React (hot reload), `/api/*` + `/ws` → `localhost:8080` |
| full / prod | Caddy container | `/` → built React files, `/api/*` + `/ws` → `api:8080` |

WebSockets need the proxy to pass through the HTTP `Upgrade` handshake — Vite needs
`ws: true` in its proxy config; Caddy does it automatically.

### In our code

| Code | What it does |
|---|---|
| [`web/vite.config.ts:18`](../web/vite.config.ts#L18) | **Dev proxy.** Vite forwards `/api/*` to Go on `localhost:8080` |
| [`web/Caddyfile:7`](../web/Caddyfile#L7) | **Full-mode proxy.** Caddy forwards `/api/*` to the `api` container |
| [`web/Caddyfile:18`](../web/Caddyfile#L18) | Everything else gets the React files (that's why `/livez` isn't exposed) |
| [`web/src/status/api.ts:17`](../web/src/status/api.ts#L17) | React calls `fetch('/api/status')` — a **relative** path, so it goes to whichever proxy served the page |

[`web/vite.config.ts:18`](../web/vite.config.ts#L18)

```ts
// web/vite.config.ts — dev mode
proxy: {
  '/api': { target: 'http://localhost:8080', changeOrigin: false },  // "/api…" → Go on your machine
  '/ws': { target: 'ws://localhost:8080', ws: true, changeOrigin: false },  // WebSockets (Part 1.3)
},
```

[`web/Caddyfile:7`](../web/Caddyfile#L7)

```
# web/Caddyfile — full mode
handle /api/* {
    reverse_proxy api:8080           # "/api/…" → the Go container (by its compose service name)
}
```

[`web/src/status/api.ts:17`](../web/src/status/api.ts#L17)

```ts
// web/src/status/api.ts — the React side
fetch('/api/status', ...)            // no host → same origin as the page
```

**See it yourself:** `make dev` → open the page → DevTools (F12) → Network → click
`status`. The request URL is `localhost:5174/api/status` — the browser never talks to `:8080`.

### Gotchas

- **CORS is not server security.** It only restrains browsers. `curl` or a script ignores
  it completely, so never rely on CORS to protect an API — that's what auth is for.
- `Access-Control-Allow-Origin: *` **cannot** be combined with credentials (cookies).
  You must echo a specific allowed origin.
- **Same-site ≠ same-origin.** `app.relay.com` and `api.relay.com` are *cross-origin* but
  *same-site* (same registrable domain `relay.com`), so `SameSite=Lax` cookies still flow
  between them. Truly different sites (`relay.com` ↔ `relay-api.io`) need `SameSite=None; Secure`.
- **A proxy may rewrite `Host`.** Vite's shorthand `'/api': 'http://localhost:8080'` sets
  `changeOrigin: true`, so Go sees `Host: localhost:8080` while the browser's `Origin` is
  `localhost:5173`. Any Origin check (our refresh endpoint, the WebSocket upgrade) then
  rejects the request as cross-origin. Found in Phase 1 when a page reload logged users out;
  fixed with `changeOrigin: false`, matching Caddy (which keeps `Host` by default).
- A proxy hides the real client IP — the app must read `X-Forwarded-For` (and only trust
  it from the proxy). Matters for rate limiting in Phase 3.

### Go deeper

- MDN: *Same-origin policy* and *Cross-Origin Resource Sharing (CORS)* — the definitive reference
- Open DevTools → Network on any big site, find an `OPTIONS` request, read its headers
- Try it in Part 0.1: temporarily call `http://localhost:8080/api/health` directly from
  React, watch the CORS error in the console, then see the proxy fix it
- web.dev: *SameSite cookies explained*

### Interview angle

- "Why does my fetch work in Postman but fail in the browser?" → SOP/CORS, Postman isn't a browser.
- "What does a reverse proxy / API gateway do in your architecture?" → TLS termination,
  routing, load balancing, rate limiting, hiding internal services.
- "How would you serve web + mobile clients?" → mobile apps aren't bound by SOP; a public
  API for third parties would need CORS (or a separate API domain).

---

## 2. Health checks: liveness vs readiness

*Came up in Part 0.1 — what `/health` should check.*

![liveness vs readiness](diagrams/notes/02-liveness-readiness.svg)

**Reading the diagram:** "copy A" and "copy B" are two identical copies of the **Go backend
only** — the frontend isn't here (it's static files served separately by Caddy/a CDN). The
two boxes inside each copy are two **URLs** on that backend, not two programs. The
**watcher** calls them; users never do.

### What it is

Something outside your app (a load balancer, Kubernetes, Docker, AWS) regularly calls
an endpoint to ask "are you OK?" — and **acts on the answer**. There are two different
questions, and they lead to two different actions:

| Probe | Question | If it fails… | Should check dependencies? |
|---|---|---|---|
| **Liveness** `/livez` | Is the process alive and not stuck? | **Restart** the server | ❌ Never |
| **Readiness** `/readyz` | Can it serve requests *right now*? | **Stop sending traffic** to it (no restart) | ✅ Yes — DB, etc. |
| *Startup* `/startupz` | Has it finished booting? | Wait; don't run the other probes yet | — |

### Why readiness checks Postgres (and liveness doesn't)

**Postgres is our database** — a separate program (in Docker) where all the app's data
lives permanently: users, chats, messages. The Go backend holds no data itself; it asks
Postgres for everything.

```
browser ──► Go backend ──► Postgres
            (logic)        (data lives here)
```

- Almost every user request needs the database. If the backend can't reach Postgres,
  it's *running* but *useless* → the honest answer to "can you take users?" is **no** →
  `/readyz` checks Postgres.
- Restarting the backend doesn't fix a broken database → `/livez` ignores Postgres.

**How we check:** a **ping** (≈ `SELECT 1`, "are you there?") with a **1-second limit**.
- Tiny on purpose: the watcher calls it every few seconds on every copy — a real query
  would load the DB just to check on it.
- The time limit matters: without it, a frozen Postgres makes the ping — and the health
  check — wait forever. With it, we always get an answer fast, even if it's "down".
- The measured time is the **latency** on the status page (`healthy (2 ms)`). If it
  climbs under load, the DB is struggling — an early warning we'll graph in Phase 2.

### Why it matters

**The classic outage:** a single `/health` that checks the database. Postgres has a
5-second blip → *every* server fails its health check at once → the orchestrator
restarts all of them (or the LB removes all of them) → a tiny DB hiccup becomes a full
outage, and the restart storm hammers the DB as it recovers.

Split probes avoid that: during the blip, servers stay alive (liveness OK), just stop
taking traffic (readiness fails), and come back on their own when the DB returns.

### What we chose

**Two endpoints** (Part 0.1, decision D):
- `/livez` → always `200 {"status":"alive"}` if the HTTP server can respond at all
- `/readyz` → pings Postgres with a short timeout (e.g. 1s);
  `200 {"status":"ready","checks":{"postgres":{"status":"ok","latency_ms":2}}}` or `503` with details
- React's status page calls `/readyz` through the proxy
- Docker Compose `healthcheck:` uses them in full mode; Phase 3's load balancer and
  Phase 7's Kubernetes will too
- `/startupz` gets added when slow startup work appears (migrations on boot)

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/health/checker.go:41`](../backend/internal/health/checker.go#L41) | `CheckPostgres` — pings the DB with a **1 s timeout** (`context.WithTimeout`, next line) |
| [`backend/internal/health/handler.go:33`](../backend/internal/health/handler.go#L33) | `Register` — which URL goes to which function |
| [`backend/internal/health/handler.go:41`](../backend/internal/health/handler.go#L41) | `livez` — checks nothing |
| [`backend/internal/health/handler.go:46`](../backend/internal/health/handler.go#L46) | `readyz` — 200 or 503 from the DB check |
| [`backend/internal/health/handler.go:55`](../backend/internal/health/handler.go#L55) | `status` — details for the UI |
| [`backend/internal/health/checker_test.go:47`](../backend/internal/health/checker_test.go#L47) | `TestCheckPostgres_PingHangs_GivesUpAtTimeout` — proves the 1-second limit works |
| [`backend/internal/health/handler_test.go:41`](../backend/internal/health/handler_test.go#L41) | `TestLivez_DBDown_StillReturns200` — this note's main rule, as a test |
| [`backend/cmd/api/main.go:141`](../backend/cmd/api/main.go#L141) | `healthcheck()` — the binary checks its own `/livez` (no curl in distroless)… |
| [`backend/internal/health/probe.go:13`](../backend/internal/health/probe.go#L13) | …using `Probe` |
| [`compose.yaml:16`](../compose.yaml#L16) | Postgres healthcheck (`pg_isready`) |
| [`compose.yaml:36`](../compose.yaml#L36) | API healthcheck (`/api healthcheck`) |

[`backend/internal/health/handler.go:41`](../backend/internal/health/handler.go#L41)

```go
// handler.go — liveness deliberately checks nothing
func (h *Handler) livez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// readiness checks the database
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.checker.CheckPostgres(r.Context()).Status != StatusOK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
```

**See it yourself:** `make dev`, then `docker compose stop postgres` and run
`curl -i localhost:8080/livez` (200) vs `curl -i localhost:8080/readyz` (503).

### Gotchas

- **Probes must be cheap and fast** — they run every few seconds on every server. `SELECT 1`, not a real query.
- **Always use a timeout** inside the check. A hanging DB call makes the probe hang,
  and the prober's own timeout produces confusing failures.
- **Don't check *other services* in readiness** unless you truly can't work without them —
  otherwise one failing service cascades into marking everything unready.
- **Graceful shutdown uses readiness:** on shutdown, first fail `/readyz` so the LB stops
  sending new requests, *then* finish in-flight requests, *then* exit. We'll do this in 0.2.
- Health endpoints can leak internals (versions, hostnames) — keep details minimal on public routes.

### Go deeper

- Kubernetes docs: *Configure Liveness, Readiness and Startup Probes*
- AWS docs: *Health checks for your target groups* (ALB) — same idea, different names
- Google SRE book, chapter *Addressing Cascading Failures* (free online)

### Interview angle

- "How does your load balancer know a server is down?" → health checks, interval,
  healthy/unhealthy thresholds, liveness vs readiness.
- "What happens to in-flight requests when you deploy?" → readiness + graceful shutdown + connection draining.
- "How do you stop one failing dependency from taking everything down?" → don't put it in liveness; circuit breakers (Phase 3+).

---

## 3. Multi-stage Docker builds & base images

*Came up in Part 0.1 — which image the Go binary runs in.*

![multi-stage build](diagrams/notes/03-multi-stage-build.svg)

### Docker basics first

```
  Dockerfile ──(docker build)──► Image ──(docker run / compose)──► Container
  a recipe                       a frozen box                      the box, running
```

- **Container** — a program running in its own sealed box, with everything it needs
  already installed inside. That's why you don't install packages yourself.
- **Image** — the frozen snapshot a container starts from. Image : container =
  class : object. One image can start many containers.
- **Where images come from:** *downloaded* ready-made (`image: postgres:18` in
  [`compose.yaml:7`](../compose.yaml#L7)) or *built* by us from a Dockerfile
  (`build: ./backend` in [`compose.yaml:24`](../compose.yaml#L24)) — nobody publishes an image of *our* app.
- **Dockerfile** — the recipe: start from X, copy code in, build it, run it.
  **`docker build`** follows the recipe and produces an image (`make up-full` does this for you).
- **Base image** — the `FROM` line: the existing image the recipe starts from, like
  starting from a project template instead of an empty folder.
- **Multi-stage** — two `FROM`s in one recipe. Stage 1 is the **kitchen** (big image
  with compilers, ~800 MB) where the app is built; stage 2 is the **lunchbox** (tiny
  image) that gets *only the finished program*. The kitchen is thrown away.

```
STAGE 1 "build" (kitchen)                STAGE 2 final (lunchbox)
FROM golang:1.27  (~800 MB)              FROM distroless (~2 MB)
copy code, go build → /api (15 MB) ────► COPY --from=build /api
kitchen thrown away ✗                    ships: ~20 MB total
```

- **Dev vs full mode:** in `make dev` only Postgres runs in Docker (Go and React run
  on your machine for instant reloads). In `make up-full` all three run in containers.

### What it is (in more depth)

**Image vs container:** an *image* is a read-only template (filesystem + startup command);
a *container* is a running instance of it. One image → many containers.

**Layers:** every Dockerfile instruction (`COPY`, `RUN`…) creates a layer. Docker caches
layers and reuses them if nothing they depend on changed. Order matters: put things that
rarely change (dependencies) *before* things that change often (your code).

**Multi-stage build:** one Dockerfile, several `FROM` stages. Early stages have heavy
tools (compiler); the final stage copies in only the result. Everything else is thrown away.

```dockerfile
# ---- stage 1: build ----
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download                 # cached until go.mod/go.sum change
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

# ---- stage 2: run ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /api /api
USER nonroot
ENTRYPOINT ["/api"]
```

**Base image options for the final stage:**

| Image | Size | Contains | Shell |
|---|---|---|---|
| `scratch` | 0 MB | nothing | ❌ |
| `distroless/static` | ~2 MB | CA certs, tzdata, `/etc/passwd` with nonroot user | ❌ |
| `alpine` | ~8 MB | musl libc, busybox shell, `apk` package manager | ✅ |
| `debian-slim` | ~75 MB | full glibc userland | ✅ |

### Why it matters

- **Smaller image** → faster pulls on deploy and autoscaling (Phase 3/7), less disk, cheaper.
- **Smaller attack surface** → no shell or package manager for an attacker to use; fewer CVEs in scanners.
- **Reproducible builds** → the same Dockerfile builds the same thing on your laptop and on AWS.

### What we chose

**`gcr.io/distroless/static:nonroot`** (Part 0.1, decision E). Go compiles to one
static binary when `CGO_ENABLED=0`, so it needs nothing from the OS except CA
certificates (to verify HTTPS servers like S3) and timezone data — exactly what distroless/static gives.

### In our code

| Code | What it does |
|---|---|
| [`backend/Dockerfile:4`](../backend/Dockerfile#L4) | Go **build** stage: `golang:1.27` |
| [`backend/Dockerfile:18`](../backend/Dockerfile#L18) | Go **run** stage: `distroless/static:nonroot`, and the next line copies in only the binary |
| [`backend/.dockerignore:1`](../backend/.dockerignore#L1) | Keeps `tmp/`, `.env` out of the build |
| [`web/Dockerfile:4`](../web/Dockerfile#L4) | React **build** stage: `node:24-alpine` |
| [`web/Dockerfile:16`](../web/Dockerfile#L16) | React **run** stage: `caddy:2-alpine` serves the built files |
| [`compose.yaml:24`](../compose.yaml#L24) | `build:` for `api`; passes the git commit as `VERSION` (line 27) |
| [`compose.yaml:46`](../compose.yaml#L46) | `build:` for `web` |

Result: `relay-api` image = **20 MB** (vs ~800 MB for the `golang` build image).

**See it yourself:** `make up-full`, then `docker images | grep relay` and
`docker history relay-api`.

### Gotchas

- **`CGO_ENABLED=0`** makes the binary fully static. Without it, Go may link to the
  system C library (glibc), and the binary crashes on images that don't have it
  (scratch, distroless/static, alpine's musl) with a confusing "not found" error.
- **`scratch` + HTTPS = `x509: certificate signed by unknown authority`** — no CA certs.
- **Layer cache busting:** `COPY . .` before `go mod download` re-downloads all
  dependencies on every code change. Copy `go.mod`/`go.sum` first.
- **`.dockerignore`** — without it, `COPY . .` sends `node_modules`, `.git`, `.env`
  (secrets!) into the build. Always add one.
- **Never run as root** in the container: if the app is compromised, root in the
  container is one kernel bug away from root on the host.
- **No shell = debug differently:** logs, metrics, or `docker debug <container>` /
  ephemeral debug containers that attach tools from outside.

### Go deeper

- Docker docs: *Multi-stage builds* and *Build cache*
- `docker history <image>` — see every layer and its size
- `dive` (open-source tool) — explore an image layer by layer
- GoogleContainerTools/distroless on GitHub — README explains each variant
- Scan an image for CVEs: `docker scout cves <image>` or `trivy image <image>`

### Interview angle

- "How do you keep container images small and secure?" → multi-stage, minimal base, non-root, `.dockerignore`, scanning.
- "Why are your deploys slow?" → huge images, bad layer ordering, no build cache.
- "Container vs VM?" → containers share the host kernel (namespaces + cgroups); VMs have their own kernel.

---

## 4. Sessions, JWTs & where tokens live

*Came up in Phase 1 — how the API knows who's calling. Full decision: [ADR 0001](adr/0001-auth-tokens.md).*

### What it is

After login the server hands the browser a **credential** it sends with every request. Two styles:

| | Session id (opaque) | JWT (self-contained) |
|---|---|---|
| Looks like | `k3J9…` random string | `header.payload.signature` (base64url) |
| Server checks it by | looking it up in a DB/Redis | verifying the signature — no lookup |
| Revoke it | delete the row — instant | can't; wait for `exp` |
| Cost per request | one lookup | one HMAC (microseconds) |

A **JWT payload is readable by anyone** (it's base64, not encryption) — the signature only proves
*we* wrote it. So: no secrets in claims.

**Where the browser keeps it** matters as much as the format:

| Storage | XSS (attacker's JS runs on our page) | CSRF (another site makes the browser call us) |
|---|---|---|
| `localStorage` | ❌ readable — stolen | ✅ not sent automatically |
| JS memory | ⚠️ usable while the page is open, gone on reload | ✅ not sent automatically |
| `HttpOnly` cookie | ✅ JS can't read it | ❌ sent automatically → needs `SameSite` / Origin checks |

### Why it matters

Every request is authenticated, so the check must be cheap; but "log this device out" and
"someone stole a token" must also be possible. Combining both styles gets both.

### What we chose

- **Access token:** JWT, 15 min, **in memory** → cheap stateless check on every request.
- **Refresh token:** random, 30 days, **`HttpOnly; SameSite=Strict; Path=/api/auth` cookie**, stored
  hashed in `sessions` → revocable. Only `/api/auth/refresh` and `/logout` ever receive it.
- **Rotation + reuse detection:** every refresh issues a new token; an old one coming back =
  a copy exists = revoke the session.
- On page load, the app calls refresh once to get a new access token.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/auth/tokens.go:42`](../backend/internal/auth/tokens.go#L42) | `IssueAccess` — the JWT's claims |
| [`backend/internal/auth/tokens.go:95`](../backend/internal/auth/tokens.go#L95) | `parse` — algorithm pinned to HS256, audience + expiry required |
| [`backend/internal/auth/middleware.go:15`](../backend/internal/auth/middleware.go#L15) | `RequireUser` — no DB query per request |
| [`backend/internal/auth/sessions.go:68`](../backend/internal/auth/sessions.go#L68) | `Rotate` — rotation and replay detection in two `UPDATE`s |
| [`backend/internal/auth/handler.go:180`](../backend/internal/auth/handler.go#L180) | the refresh cookie's flags, each with its reason |
| [`backend/internal/auth/handler.go:203`](../backend/internal/auth/handler.go#L203) | `checkOrigin` — CSRF defence in depth |
| [`web/src/lib/api.ts:42`](../web/src/lib/api.ts#L42) | `api()` — on 401, refresh once and retry |
| [`web/src/lib/api.ts:94`](../web/src/lib/api.ts#L94) | `refreshSession` — one in-flight refresh, Web Locks across tabs |
| [`backend/internal/auth/tokens_test.go:49`](../backend/internal/auth/tokens_test.go#L49) | the `alg: none` attack, as a test |
| [`backend/internal/auth/sessions_test.go:14`](../backend/internal/auth/sessions_test.go#L14) | replaying an old refresh token revokes the session |

**See it yourself:** log in, open DevTools → Application → Cookies: `relay_refresh` is there,
marked HttpOnly — then type `document.cookie` in the console: it's not visible. Paste an
access token (Network tab → any `/api` request → `Authorization`) into jwt.io and read its claims.

### Gotchas

- **Never trust the token's `alg` header.** Libraries that do accept `alg: none` (no signature)
  or let an RS256 public key be used as an HS256 secret. Pin the algorithm.
- **Tabs share cookies.** Two tabs refreshing at the same moment send the same token twice —
  rotation calls that theft. We serialise with `navigator.locks` (which only exists on https
  and localhost; on a plain-http LAN address the in-tab guard is all you get).
- **JWT logout is a lie for up to `exp`.** Keep access tokens short, or keep a revocation list.
- `SameSite=Strict` cookies aren't sent when arriving from a link on another site — fine for a
  refresh cookie, wrong for a session cookie on a site people link to.

### Go deeper

- RFC 8725 *JSON Web Token Best Current Practices* (short, practical)
- OWASP *Session Management Cheat Sheet* and *JSON Web Token Cheat Sheet*
- Auth0 blog: *Refresh Token Rotation* (the reuse-detection idea)

### Interview angle

- "JWT or sessions?" → both: stateless short access + stateful long refresh; explain revocation.
- "Where do you store tokens in an SPA?" → the XSS vs CSRF table above.
- "How do you log a user out everywhere?" → revoke sessions + short access TTL or a denylist by `sid`.

---

## 5. HTTP vs polling vs WebSockets

*Came up in Phase 1 — how a message reaches the other person instantly. Decision: [ADR 0003](adr/0003-websocket-protocol.md).*

### What it is

HTTP is **request → response**: the server can only answer, never start a conversation.
To get "the server tells me when something happens":

| Technique | How | Latency | Cost |
|---|---|---|---|
| Short polling | ask every N seconds | up to N s | many empty requests |
| Long polling | ask; server holds the request until there's news (or ~30 s) | low | one held request per client, re-opened after each answer |
| Server-Sent Events | one long HTTP response the server keeps writing to | low | server → client only |
| **WebSocket** | HTTP request with `Upgrade: websocket`, then the TCP connection becomes a two-way message pipe | lowest | one open connection per client = **state on the server** |

### Why it matters

A WebSocket makes the server **stateful**: it must remember who is connected *on this
process*. That's easy with one server and is exactly what breaks with two (Phase 3: user A on
server 1 can't reach user B on server 2 without something like Redis pub/sub).

### What we chose

- WebSocket for server → client **events only**; every write stays a REST call.
- Short-lived ticket for auth (browsers can't set headers on `new WebSocket()`).
- One goroutine per connection writing from a bounded outbox; a slow client gets
  disconnected instead of slowing everyone down.
- Ping/pong every 25 s detects dead connections; reconnect uses exponential backoff with jitter.
- Every (re)connect is followed by a catch-up fetch, because the socket can always miss something.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/realtime/hub.go:80`](../backend/internal/realtime/hub.go#L80) | `Publish` — encode once, never block, evict slow clients |
| [`backend/internal/realtime/handler.go:51`](../backend/internal/realtime/handler.go#L51) | `serve` — ticket, upgrade, writer loop |
| [`backend/internal/realtime/handler.go:65`](../backend/internal/realtime/handler.go#L65) | `websocket.Accept` checks `Origin` |
| [`backend/internal/realtime/handler.go:96`](../backend/internal/realtime/handler.go#L96) | heartbeat: ping, give up after 10 s |
| [`backend/internal/chat/service.go:355`](../backend/internal/chat/service.go#L355) | publish only **after** the transaction commits |
| [`web/src/realtime/socket.ts:13`](../web/src/realtime/socket.ts#L13) | `backoffDelay` — 0.5 s → 30 s, with jitter |
| [`web/src/realtime/events.ts:43`](../web/src/realtime/events.ts#L43) | `catchUp` — runs on every open |
| [`web/vite.config.ts:19`](../web/vite.config.ts#L19), [`web/Caddyfile:12`](../web/Caddyfile#L12) | the proxies pass the `Upgrade` through |

**See it yourself:** DevTools → Network → filter "WS" → click `/ws` → Messages. Send a message
from another browser and watch the `message.created` frame arrive.

### Gotchas

- **Publish after commit**, or clients see messages that then roll back.
- **Jitter**: without it, a server restart makes every client reconnect at the same millisecond.
- **The first connect needs catch-up too**: anything sent between "page fetched history" and
  "socket open" is otherwise lost.
- Browsers can't send WebSocket pings or see a silently dead connection; only the server can
  ping. Playwright's `setOffline` doesn't even close a localhost socket (found in our e2e tests).
- Server `WriteTimeout` doesn't apply after the upgrade: `net/http` clears deadlines on hijack.

### Go deeper

- RFC 6455 sections 1 and 5 (the handshake and framing — short and readable)
- *The WebSocket Handbook* (Ably, free) — scaling chapter
- Slack engineering: *Real-time Messaging* (how they fan out over WebSockets)

### Interview angle

- "Design WhatsApp/Slack" → connection gateway, fan-out, offline catch-up, ordering, idempotency.
- "Why not long polling?" → overhead per message, but it works through every proxy — a fallback.
- "How do you scale WebSockets?" → stateful gateways + pub/sub between them (Phase 3).

---

## 6. Ordering & cursor pagination

*Came up in Phase 1 — message order, history, and catching up. Decision: [ADR 0002](adr/0002-chat-schema.md).*

### What it is

**OFFSET pagination:** `ORDER BY seq DESC LIMIT 50 OFFSET 5000` — the database still walks
and throws away 5000 rows, and if 3 new messages arrive between pages, everything shifts:
you see duplicates.

**Cursor (keyset) pagination:** `WHERE seq < 5012 ORDER BY seq DESC LIMIT 50` — "the 50 before
the last one I have". With an index on `(chat_id, seq)` it's a range scan: page 1 and page
10,000 cost the same, and new rows don't shift anything.

**Ordering:** a cursor is only as good as its order. An auto-increment id is assigned when a
transaction *starts writing*, not when it *commits*, so ids can become visible out of order:

```
tx A: gets id 10 ............................ COMMIT
tx B:       gets id 11 ... COMMIT
reader:                         sees 11 → asks "after 11" → never sees 10
```

### What we chose

A **per-chat `seq`** handed out by `UPDATE chats SET last_seq = last_seq + 1 … RETURNING` inside
the send transaction. The row lock queues senders *per chat*, so commit order = seq order and
"after N" is always complete. Pages are cursors on `seq` (`before` to scroll up, `after` to
catch up), fetching `limit + 1` rows to know `has_more` without `COUNT`. Unread is
`last_seq − last_read_seq`: no counting at all.

### In our code

| Code | What it does |
|---|---|
| [`backend/migrations/0002_chats.up.sql:38`](../backend/migrations/0002_chats.up.sql#L38) | `UNIQUE (chat_id, seq)` — the index every page uses |
| [`backend/internal/chat/store.go:299`](../backend/internal/chat/store.go#L299) | `Send` — idempotency check, then… |
| [`backend/internal/chat/store.go:310`](../backend/internal/chat/store.go#L310) | …the row lock that orders sends |
| [`backend/internal/chat/store.go:376`](../backend/internal/chat/store.go#L376) | `History` — `before` / `after` cursors, `limit + 1` |
| [`backend/internal/chat/store.go:173`](../backend/internal/chat/store.go#L173) | `ListForUser` — unread by subtraction, one query |
| [`backend/internal/chat/service.go:96`](../backend/internal/chat/service.go#L96) + [`users/store.go:83`](../backend/internal/users/store.go#L83) | the N+1 fix: collect ids, one `GetMany` |
| [`backend/internal/chat/store_test.go:96`](../backend/internal/chat/store_test.go#L96) | 20 concurrent sends → seq 1…20, no gaps |
| [`web/src/chat/queries.ts:42`](../web/src/chat/queries.ts#L42) | the client's cursor: oldest seq on the last page |
| [`web/src/chat/cache.ts:26`](../web/src/chat/cache.ts#L26) | `upsertMessage` — dedupe and order by seq |

**See it yourself:** `make psql`, then
`EXPLAIN ANALYZE SELECT * FROM messages WHERE chat_id = 1 AND seq < 100 ORDER BY seq DESC LIMIT 51;`
→ an *Index Scan* on `messages_chat_id_seq_key`. Try `OFFSET 50000` on a big chat to compare.

### Gotchas

- The cursor column must be **unique** (or use a tuple like `(created_at, id)`).
- Timestamps are a bad cursor: ties, and clocks differ between servers.
- A row lock per chat is a throughput ceiling for *one* chat — fine for chat rooms, not for a
  live stream with thousands of writers (Phase 6).
- Soft-deleting keeps seq gapless, so clients can tell "deleted" from "missing".

### Go deeper

- Markus Winand, *use-the-index-luke.com* → "Paging Through Results" (and his "no offset" talk)
- PostgreSQL docs: *Explicit Locking* → row-level locks
- Designing Data-Intensive Applications, ch. 7 (transactions) and ch. 9 (ordering)

### Interview angle

- "Paginate an infinite feed" → keyset pagination; why OFFSET degrades.
- "How do you guarantee message order?" → per-conversation sequence; total order is expensive and unnecessary.
- "How does a client know what it missed?" → monotonically increasing cursor + "after" query.

---

## 7. Password hashing & CPU-heavy work

*Came up in Phase 1 — decision A, then the Part 1.4 load test. Results: [1.4 learnings](phases/phase-1/1.4-measure/learnings.md).*

### What it is

A password hash must be **slow on purpose**: if the database leaks, each guess should cost an
attacker real CPU time. bcrypt has a **cost** (work factor): cost 12 = 2¹² rounds ≈ 250 ms of
one core. Each hash includes a random **salt**, so equal passwords get different hashes and
precomputed tables are useless. (Argon2id is the modern choice: also memory-hard, which hurts GPUs.)

Fast hashes (SHA-256) are right for **random tokens** — 256 random bits can't be guessed no
matter how fast you hash — and wrong for passwords.

### Why it matters

"Slow on purpose" is a **capacity problem**. Our back-of-envelope said 100 logins at once ≈
25 CPU-seconds, more than all the chatting. Part 1.4 measured it: a burst of 50 logins took
all 4 cores for ~3 s, and **every message sent during that time waited up to 3.3 s**, even
though messages need ~1 ms of CPU. Go shares CPU fairly between goroutines, so a send that
waits on the DB several times rejoins the back of a queue of 50 bcrypt goroutines each time.

### What we chose

bcrypt cost 12 + a **bulkhead**: at most `GOMAXPROCS − 1` hashes run at once (a buffered
channel used as a semaphore); the rest queue, and one core always stays free for chat. A login
queued longer than 10 s gets `503` + `Retry-After` (**load shedding**) instead of piling up.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/auth/password.go:19`](../backend/internal/auth/password.go#L19) | `ValidatePassword` — bcrypt's 72-byte limit |
| [`backend/internal/auth/password.go:60`](../backend/internal/auth/password.go#L60) | `Hash` — runs inside a slot |
| [`backend/internal/auth/password.go:83`](../backend/internal/auth/password.go#L83) | `VerifyNobody` — same time for unknown emails |
| [`backend/internal/auth/password.go:91`](../backend/internal/auth/password.go#L91) | `inSlot` — the semaphore, timeout and cancel |
| [`backend/internal/app/app.go:56`](../backend/internal/app/app.go#L56) | `hashSlots` — GOMAXPROCS − 1 |
| [`backend/internal/auth/password_test.go:88`](../backend/internal/auth/password_test.go#L88) | at most N hashes at once, as a test |
| [`backend/internal/auth/password_test.go:50`](../backend/internal/auth/password_test.go#L50) | timing: unknown email ≈ known email |

### Gotchas

- bcrypt silently **truncates at 72 bytes** — reject longer passwords (or pre-hash).
- Limits in characters vs bytes: `é` is 2 bytes.
- `runtime.NumCPU()` ignores container CPU limits; `GOMAXPROCS` respects them (Go 1.25+).
- A bulkhead trades the slowest login's latency for everyone else's: in our test login p95 went
  3.45 s → 4.38 s while the *average* login got faster (2.41 s): queueing finishes early arrivals
  sooner instead of making everyone share CPU and finish together.

### Go deeper

- OWASP *Password Storage Cheat Sheet* (current parameters for Argon2id/bcrypt)
- *Release It!* (Michael Nygard) — Bulkheads, Shed Load, Fail Fast
- Go blog / docs: the scheduler and `GOMAXPROCS` in containers

### Interview angle

- "How do you store passwords?" → slow salted hash, cost tuning, pepper, rehash on login.
- "One expensive endpoint is slowing everything" → bulkhead/semaphore, separate pool, queue + shed, or move it to its own service.
- "Why did p99 jump while average looked fine?" → contention for a shared resource; find it with a profiler, not by guessing.

---

## 8. Temporary vs stored events, and write amplification

*Came up in Part 2.1 — typing indicators and read receipts.*

### What it is

Not every event deserves a database row:

| | Temporary event | Stored event |
|---|---|---|
| Example | "Alice is typing" | a message, a read marker, a reaction |
| If it's lost | nobody notices — the next one replaces it | the conversation is wrong |
| Lives | in memory, for seconds | in Postgres, forever |
| Delivered | best effort over the WebSocket | written first, *then* pushed; caught up on reconnect |

**Write amplification:** one user action causes many writes or deliveries. A read receipt is
one `UPDATE` — then one event to *every* member. In a group of 50 where everyone reads every
message, one message causes 50 reads × 50 recipients = **2,500 event deliveries**.

### Why it matters

Storing typing events would mean a database write per keystroke burst for something nobody
will ever read back. And amplification is how "a small feature" becomes the biggest load: in
our 100-user soak test, **read calls outnumbered messages** (2,494 vs 1,987 in 3 minutes) and
clients received ~4 WebSocket events for every message delivered.

### What we chose

- Typing: a client → server WebSocket frame, membership-checked, relayed to the other members,
  never stored; the client sends at most one per 3 s and shows it for 6 s.
- A per-connection **token bucket** (burst 5, 2/s) drops floods — a fixed "one frame per 500 ms"
  rule dropped real typing events (see [2.1 learnings](phases/phase-2/2.1-richer-chat/learnings.md)).
- Read receipts reuse `last_read_seq` (no per-message rows); the client sends at most one read
  per newest message.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/chat/service.go:473`](../backend/internal/chat/service.go#L473) | `Typing` — check membership, publish, store nothing |
| [`backend/internal/realtime/handler.go:142`](../backend/internal/realtime/handler.go#L142) | `read` — the client-frame loop and its rate limit |
| [`backend/internal/realtime/bucket.go:21`](../backend/internal/realtime/bucket.go#L21) | the token bucket |
| [`backend/internal/chat/service.go:430`](../backend/internal/chat/service.go#L430) | `MarkRead` — one write, one event per member |
| [`web/src/realtime/typing.ts:21`](../web/src/realtime/typing.ts#L21) | typing entries that expire on their own |
| [`web/src/chat/receipts.ts:6`](../web/src/chat/receipts.ts#L6) | ✓/✓✓ and "Seen by N" derived from `last_read_seq` |

### Gotchas

- Rate limits on *temporary* events can be strict, but not so strict that normal use trips
  them: test the "send, then immediately type again" path.
- Fan-out cost grows with group size squared for reads. If it ever matters: batch read events,
  or send receipts only to the message's sender.

### Go deeper

- *Designing Data-Intensive Applications*, ch. 11 (stream processing) — events vs state
- WhatsApp/Slack engineering posts on typing indicators and read receipts at scale

### Interview angle

- "Design typing indicators" → ephemeral, best-effort, throttled, never persisted.
- "Read receipts in a 1,000-person group?" → amplification; aggregate counts, lazy fetch, or receipts only to the sender.

---

## 9. Metrics: RED, USE and label cardinality

*Came up in Part 2.4 — what to measure, and how not to break Prometheus.*

### What it is

Two checklists cover almost everything:

| Method | For | Measure |
|---|---|---|
| **RED** | every service / endpoint | **R**ate (req/s), **E**rrors (failures/s), **D**uration (latency histogram) |
| **USE** | every resource (CPU, pool, queue) | **U**tilisation, **S**aturation (waiting), **E**rrors |

A **histogram** counts requests into latency buckets (≤ 5 ms, ≤ 10 ms, …); Prometheus computes
p95/p99 from the buckets at query time — averages hide the slow tail.

**Cardinality** = the number of distinct label combinations. Each one is a separate time series
held in memory. A label like `path="/api/chats/8121"` makes a new series per chat; 100k chats ×
13 buckets × methods… and Prometheus runs out of memory.

### What we chose

- RED per route **pattern** (`/api/chats/{id}`, taken from Go's `ServeMux`), never the raw path;
  unmatched routes become `"unmatched"`.
- USE for the DB pool (in use / idle / max, waits) and the bcrypt bulkhead (slots in use, queue wait).
- WebSocket gauges and per-type event counters; Go runtime and process metrics for free.
- `/metrics` on a private port that only Prometheus (inside Docker) can reach.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/metrics/metrics.go:85`](../backend/internal/metrics/metrics.go#L85) | `ObserveHTTP` — the RED metrics |
| [`backend/internal/httpx/middleware.go:82`](../backend/internal/httpx/middleware.go#L82) | `Route` — records the mux pattern, innermost wins |
| [`backend/internal/httpx/middleware.go:38`](../backend/internal/httpx/middleware.go#L38) | `Log` — request ID, one log line, metrics |
| [`backend/internal/metrics/metrics.go:124`](../backend/internal/metrics/metrics.go#L124) | the DB pool, read fresh on every scrape |
| [`backend/internal/app/metrics_test.go:21`](../backend/internal/app/metrics_test.go#L21) | a test that no raw path leaks into a label |
| [`deploy/monitoring/prometheus/prometheus.yml:9`](../deploy/monitoring/prometheus/prometheus.yml#L9) | what Prometheus scrapes |

**See it yourself:** `make up-full-monitoring`, open http://localhost:3000, use the app, watch
"Relay — overview". Explore → Prometheus: `histogram_quantile(0.95, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))`.

### Gotchas

- Never put ids, emails or free text in labels; put them in **logs** (with the request ID).
- `rate()` needs a range at least ~4× the scrape interval (`[1m]` for 15 s scrapes).
- A WebSocket "request" lasts hours — keep it out of latency histograms.
- p95 of averages ≠ average of p95s: aggregate buckets (`sum by (le)`) *then* take the quantile.

### Go deeper

- Brendan Gregg, *The USE Method*; Tom Wilkie, *The RED Method*
- Prometheus docs: *Histograms and summaries*, *Naming practices*
- Google SRE book, ch. 6 *Monitoring Distributed Systems* (the four golden signals)

### Interview angle

- "How do you know your service is healthy?" → golden signals / RED, SLOs on p95/p99.
- "Prometheus is using 20 GB of RAM" → cardinality explosion; find the label.

---

## 10. Object storage & presigned uploads

*Came up in Part 2.2 — profile pictures. Decision: [ADR 0004](adr/0004-object-storage-uploads.md).*

### What it is

**Object storage** (S3 and compatibles) stores blobs under keys in buckets, over HTTP. No
folders, no partial updates — put, get, delete whole objects — which is what makes it cheap,
durable (S3: 11 nines) and endlessly scalable.

A **presigned request** is permission in a URL: the server signs "PUT this key, valid 5 minutes"
with its credentials; anyone holding it can do exactly that, nothing more. A **presigned POST
policy** goes further: it's a signed JSON document of *conditions* — key, content type,
`content-length-range` — that S3 checks against the upload.

### Why it matters

Uploads through the API cost our CPU, memory and a request slot per byte; files in the database
bloat every backup. Direct-to-storage uploads take the API out of the data path completely.

### What we chose

Browser shrinks the picture → asks the API for a POST policy (≤ 1 MiB, JPEG/PNG/WebP, one random
key) → posts straight to storage → asks the API to confirm, which checks what's actually there.
Pictures are served by the API under `/media/` with immutable caching. In AWS the API signs with
the instance role's temporary credentials — no access keys anywhere.

### In our code

| Code | What it does |
|---|---|
| [`backend/internal/storage/storage.go:93`](../backend/internal/storage/storage.go#L93) | `PresignUpload` — the policy and its conditions |
| [`web/src/profile/avatar.ts:14`](../web/src/profile/avatar.ts#L14) | resize in a `<canvas>` |
| [`web/src/profile/avatar.ts:27`](../web/src/profile/avatar.ts#L27) | the form: policy fields first, file **last** |
| [`backend/internal/users/avatar.go:79`](../backend/internal/users/avatar.go#L79) | `confirm` — trust storage, not the client |
| [`backend/internal/users/avatar.go:140`](../backend/internal/users/avatar.go#L140) | `serve` — ETag, immutable cache, `nosniff` |
| [`backend/internal/users/store.go:184`](../backend/internal/users/store.go#L184) | `SetAvatar` — swap keys atomically, return the old one |
| [`deploy/terraform/storage.tf:39`](../deploy/terraform/storage.tf#L39) | the bucket's CORS rule: only our origin may POST |
| [`deploy/terraform/iam.tf:17`](../deploy/terraform/iam.tf#L17) | the role's policy: `avatars/*` in one bucket |

### Gotchas

- A presigned **PUT** can't limit size; POST policies can.
- In a multipart S3 upload the **file must be the last field** — fields after it are ignored.
- Serving user uploads from your own origin is an XSS risk: never accept SVG/HTML, always `nosniff`.
- Presigned URLs signed with temporary credentials die when those credentials expire.
- Postgres: a `FOR UPDATE` CTE referenced only in `RETURNING` runs *after* the update and skips the
  row it changed — join it in `FROM` (found while writing `SetAvatar`).

### Go deeper

- AWS docs: *Browser-based uploads using POST (AWS Signature Version 4)*
- *Designing Data-Intensive Applications*, ch. 3 (storage engines) for why blobs and rows differ

### Interview angle

- "Design image upload for Instagram" → presigned direct-to-S3, async processing, CDN, immutable keys.
- "Why not store images in the database?" → backups, cache pressure, cost per GB, throughput.

---

## 11. One server on the internet: VPC, firewall, IAM, TLS

*Came up in Part 2.3 — the first AWS deploy. Decision: [ADR 0005](adr/0005-single-instance-deploy.md), runbook: [deploy/README.md](../deploy/README.md).*

### What it is

| Piece | One-line meaning |
|---|---|
| **VPC** | your private network in AWS (here 10.0.0.0/16) |
| **Subnet** | a slice of it in one availability zone; *public* = its route table points 0.0.0.0/0 at an internet gateway |
| **Security group** | a stateful firewall on the instance: list what may come in |
| **Elastic IP** | a public IPv4 that stays yours across stop/start |
| **IAM role** | an identity for the instance; code on it gets short-lived credentials from the metadata service |
| **DNS A record** | name → IP; `1-2-3-4.sslip.io` is a free service that answers with the IP in the name |
| **TLS (HTTPS)** | encryption + proof the server owns the name; Let's Encrypt issues free 90-day certificates via the ACME protocol, Caddy renews them |

### Why it matters

Each line is a door. Get one wrong and you're either unreachable (missing route, closed port)
or exposed (SSH open to the world, credentials in a file, a public bucket).

### What we chose

443/80 from anywhere, 22 from your IP only; no NAT gateway; IMDSv2 with hop limit 2; an IAM role
instead of keys; Caddy for TLS; `cpu_credits = "standard"` and a zero-spend budget so mistakes
show up as slowness or an e-mail, not a bill.

### In our code

| Code | What it does |
|---|---|
| [`deploy/terraform/network.tf:28`](../deploy/terraform/network.tf#L28) | the route that makes the subnet public |
| [`deploy/terraform/security.tf:8`](../deploy/terraform/security.tf#L8) | SSH from your IP only |
| [`deploy/terraform/instance.tf:28`](../deploy/terraform/instance.tf#L28) | standard CPU credits — throttle, don't bill |
| [`deploy/terraform/instance.tf:36`](../deploy/terraform/instance.tf#L36) | IMDSv2 + hop limit 2 |
| [`deploy/terraform/instance.tf:64`](../deploy/terraform/instance.tf#L64) | the free sslip.io name |
| [`deploy/terraform/budget.tf:3`](../deploy/terraform/budget.tf#L3) | the zero-spend alert |
| [`web/Caddyfile.prod:8`](../web/Caddyfile.prod#L8) | automatic HTTPS for `{$DOMAIN}` |
| [`web/Caddyfile.prod:15`](../web/Caddyfile.prod#L15) | Content-Security-Policy and friends |
| [`scripts/deploy.sh:50`](../scripts/deploy.sh#L50) | shipping images without a registry |

### Gotchas

- t3 instances default to **unlimited** CPU credits: sustained CPU above baseline is billed.
- Containers can't reach the instance metadata service with the default hop limit of 1 → S3 403s.
- Let's Encrypt rate-limits certificates per domain per week: keep Caddy's data volume.
- Postgres reads `POSTGRES_PASSWORD` only when its volume is created; changing it later does nothing.
- Every public IPv4 address is billable since 2024 (covered by the free tier's 750 h, i.e. one).

### Go deeper

- AWS docs: *How Amazon VPC works*; *Burstable performance instances* (credits)
- *How HTTPS works* (comic, howhttps.works); Let's Encrypt docs: *How it works*
- Julia Evans, *Networking! ACK!* zine

### Interview angle

- "Walk me through what happens when I type the URL" → DNS, TCP, TLS handshake, proxy, app.
- "How do your servers get cloud credentials?" → instance roles + metadata service, never baked-in keys; SSRF and IMDSv2.

