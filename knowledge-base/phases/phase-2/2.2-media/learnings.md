# Part 2.2 — Profile pictures: Learnings

Spec: [design.md § 3](../design.md#section-3--part-22-profile-pictures-object-storage) · Decision: [ADR 0004](../../../adr/0004-object-storage-uploads.md) ·
Concept: [Note 10](../../../Notes.md#10-object-storage--presigned-uploads)

## What was built

| Piece | Where | Read this first |
|---|---|---|
| S3 client (presigned POST, stat, open, delete) | `backend/internal/storage/` | `PresignUpload` |
| Upload → confirm → serve | `backend/internal/users/avatar.go` | `confirm` (trust storage, not the browser) |
| `users.avatar_key`, `avatar_url` on every profile | `migrations/0004_avatars.up.sql`, `users/store.go` | `SetAvatar` |
| RustFS locally, `/storage` and `/media` routes | `compose.yaml`, `web/vite.config.ts`, `web/Caddyfile` | |
| Resize in the browser, upload, profile page | `web/src/profile/avatar.ts`, `ProfilePage.tsx` | `uploadForm` |

Tests: `internal/storage` against real RustFS (size and type limits enforced by storage),
`internal/users/avatar_test.go` (fake storage: the whole flow, claiming others' keys, SVG,
lying content types), web unit tests (form order, flow), e2e `avatar.spec.ts` (real browser,
real canvas resize, real RustFS — in dev, full mode and the production stack).

## Already observed while building

- **MinIO is gone from Docker Hub** (they stopped publishing community images). RustFS speaks the
  same S3 API including POST policies — checked first with a 30-line probe: a valid upload → 204,
  5 KB against a 1 KB limit → `EntityTooLarge`, wrong type → `InvalidPolicyDocument`.
- **A Postgres surprise:** `WITH old AS (SELECT … FOR UPDATE) UPDATE … RETURNING (SELECT … FROM old)`
  returned NULL for the old key. A locking CTE that's only referenced in `RETURNING` runs after the
  update and skips the row the same statement changed. Joining the CTE in `FROM` fixes it — a test
  caught it before any picture was orphaned.
- **Two AWS-only bugs caught without AWS**, by testing what the code *would* do:
  `minio-go` builds S3's dual-stack host (`bucket.s3.dualstack.<region>…`), which the CSP and the
  CORS origin from Terraform don't name — the browser would have blocked every upload. Now pinned
  by a test. And an IAM `s3:ListBucket` grant with an `s3:prefix` condition never applies to
  HEAD/GET requests, so a missing object would have come back as 403 instead of 404.
- Uploading a 4 MB phone photo sends ~20 KB: the browser crops and re-encodes to 256×256 JPEG.
- `/media/…` answers `304 Not Modified` to a repeat with `If-None-Match`, and browsers rarely even
  ask: `immutable, max-age=1 year`.

## Experiments to run

### 1. Try to break the policy
Get an upload policy (`POST /api/me/avatar/upload` in DevTools → copy as curl), then use `curl -F`
to post (a) a 2 MB file, (b) the right file with `Content-Type: text/html`, (c) after 6 minutes.
What does storage answer each time?

**Your results:**

### 2. Claim someone else's picture
Confirm a key that starts with another user's id. Then one with `..` in it. What stops you?

**Your results:**

### 3. Where are the bytes?
`make psql` → `SELECT username, avatar_key FROM users;`, then look at the files:
`docker compose exec storage ls -R /data/relay-media`. Delete your picture in the app — is the
file gone? Upload a new one without confirming it (stop after step 3 in DevTools): what's left behind?

**Your results:**
