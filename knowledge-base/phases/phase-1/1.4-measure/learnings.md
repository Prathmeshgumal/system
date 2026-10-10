# Part 1.4 — Measure it: Learnings

Spec: [design.md § 6](../design.md#section-6--part-14-measure-it) · Concept: [Note 7](../../../Notes.md#7-password-hashing--cpu-heavy-work)

## What was built

| Piece | Where |
|---|---|
| k6 test: 100 users, login burst, delivery latency | `loadtest/chat.js` (`make loadtest`) |
| pprof on a private address | `backend/cmd/api/main.go` `servePprof` (`PPROF_ADDR=127.0.0.1:6060`) |
| The fix: bcrypt bulkhead + load shedding | `backend/internal/auth/password.go` `inSlot`, `internal/app/app.go` `hashSlots` |

## The test

Setup signs up 100 users and makes 20 groups of 5 (each group mixes early and late users).
Users 1–50 log in and start chatting at t = 0; **users 51–100 all log in at t = 30 s** while
the first 50 are chatting; everyone sends a message every 3 s (≈ 10× the design estimate)
until t = 120 s. Each message carries its send time, so every receiver measures delivery
latency: sender's POST → event on the receiver's WebSocket.

Machine: 4 vCPU, 15 GB, Postgres 16 and the API on the same box, `BCRYPT_COST=12`.

## Prediction (from the design's back-of-envelope)

> Chat traffic is tiny. The first bottleneck is bcrypt during the login burst, not messaging.

## Run 1 — before

All thresholds passed (0 errors, delivery p95 15 ms) — but the maxima told another story:

| Metric | median | p95 | max |
|---|---|---|---|
| send (HTTP) | 4.8 ms | 14.9 ms | **3,323 ms** |
| delivery | 5 ms | 15 ms | **3,323 ms** |
| login | 3,353 ms | 3,457 ms | 3,462 ms |
| WebSocket connect | 1.6 ms | 146 ms | 226 ms |

The API log, bucketed by second: sends over 300 ms happened **only** in the second the 50
late logins arrived — and *all 50* sends in that second were slow.

CPU profile (`go tool pprof -top`), whole run:

```
 50.33s 91.08%  golang.org/x/crypto/blowfish.encryptBlock
 26.18s 47.38%  (cum) auth.(*Handler).login
 26.01s 47.07%  (cum) auth.(*Handler).signup
```

**94% of all CPU was bcrypt.** All the chatting — 3,400 sends and 12,600 deliveries — used under 3%.

**Why a 1 ms send waited 3 s:** 50 goroutines each wanted ~250 ms of CPU on 4 cores. Go's
scheduler shares CPU fairly, so the send — which pauses several times waiting for Postgres —
rejoins the back of that queue after every pause.

## The fix — a bulkhead

Let at most `GOMAXPROCS − 1` (here 3) hashes run at once; other logins queue for a slot; one
core always stays free for everything else. A login that would queue more than 10 s gets
`503 busy` + `Retry-After: 2` instead (load shedding). Not chosen: lowering bcrypt cost
(weakens every password to fix a burst that happens once a morning).

## Run 2 — after (same test)

| Metric | before | after |
|---|---|---|
| send p95 / **max** | 14.9 / **3,323 ms** | 7.4 / **30 ms** |
| delivery p95 / **max** | 15 / **3,323 ms** | 8 / **30 ms** |
| delivery during the login burst, max | 722 ms | **11 ms** |
| WebSocket connect p95 | 146 ms | 1.6 ms |
| login median / p95 / max | 3.35 / 3.46 / 3.46 s | **2.46** / 4.39 / 4.62 s |
| errors | 0 | 0 |

**The trade-off:** the slowest login waits ~1 s longer, but the *median* login got faster.
With fair sharing all 50 finish together at ~3.4 s; with a queue the first ones finish in
~0.3 s and only the last ones wait. And nobody's messages stall.

## Lessons

1. **Look at the max, not just p95.** Run 1 passed every threshold; the stall was 1 second
   in 120, invisible to p95 and obvious in max and in a per-second log breakdown.
2. **The estimate was right** — the habit pays: we knew where to look before running anything.
3. **CPU-heavy work and latency-sensitive work shouldn't share a pool unguarded.** Bulkheads
   are the cheapest isolation; a separate service is the expensive one (cf. the split rules
   in [monolith → microservices](../../../concepts/monolith-to-microservices.md): "fails differently").
4. One API server on 4 cores carried 100 active users at ~26 msg/s with p95 8 ms. The next
   limit is somewhere far above v0.1's target — Phase 2.4 finds it on a t3.micro.

## Experiments to run

1. **Reproduce:** `make dev` (with `PPROF_ADDR=127.0.0.1:6060` in `.env`), then `make loadtest`.
   While it runs: `go tool pprof -http=:7070 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'`
   and open the flame graph. Where's the widest bar?
2. **Turn the fix off:** in `internal/app/app.go` make `hashSlots` return 100. Re-run. Do you
   see the stall? Then try returning 1.
3. **Make the burst bigger:** set both scenarios to 100 VUs (200 users; raise `USERS`). Do any
   logins get `503 busy`? What does the client do with it today? (Hint: it shows an error;
   a retry-after-wait would be better.)
4. **Without the profiler:** how would you have found this from the logs alone?

**Your results:**
