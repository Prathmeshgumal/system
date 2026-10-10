# 0004 — Profile pictures: S3-compatible storage, presigned POST uploads, served through the API

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 2 (Part 2.2)

## Context
Users upload a profile picture. Files must not live in Postgres (they bloat every backup and
compete with chat queries for memory), must not be able to take over the API (a 50 MB upload
shouldn't tie up a request goroutine and a DB connection), and must cost $0: AWS S3 in
production, something S3-compatible locally. The browser and the API are on one origin.

## Options considered
**Where files go**
1. Postgres `bytea` — one system to run; but every backup carries every picture, and big rows
   hurt the cache. ❌
2. The server's disk — simple; but lost with the instance, and Phase 3 has several servers. ❌
3. **Object storage via the S3 API** — AWS S3 in production; locally **RustFS** (MinIO stopped
   publishing Docker images in 2025; RustFS implements the same API, incl. POST policies).

**How bytes get there**
1. Through the API (`multipart/form-data` to Go, Go writes to S3) — easy to validate, but every
   byte passes through our one small server.
2. **Presigned PUT** URL — direct to storage, but a PUT URL can't limit the size.
3. **Presigned POST policy** — direct to storage, and the *policy* (signed by the API) says: this
   key, this content type, 1 byte to 1 MiB, valid 5 minutes. Storage enforces it.

**How pictures are read**
1. Public bucket — fastest, but the bucket becomes public and URLs are bucket URLs.
2. Presigned GET per view — private, but URLs change every time, so nothing caches.
3. **`GET /media/{key}` served by the API**, keys random and never reused → `Cache-Control:
   immutable, max-age=1 year`. Private bucket, same origin, and a CDN can sit in front later.

## Decision
Presigned **POST** straight from the browser, then a **confirm** call
(`PUT /api/me/avatar {key}`) where the API checks the object really exists, is ≤ 1 MiB and is
JPEG/PNG/WebP before pointing the user at it; the old picture is deleted. Reads go through
`/media/`. The browser shrinks pictures to 256×256 JPEG first (~20 KB). In AWS the API has
**no keys**: the instance's IAM role allows `avatars/*` in one bucket.

## Consequences
- ✅ Upload bytes never touch the API or the database; size/type limits enforced by storage.
- ✅ The same code talks to S3 and RustFS (`minio-go`), configured by `S3_*` settings.
- ✅ SVG is never accepted, and `/media` sends `nosniff`: an uploaded file can't become a script
  on our origin.
- ❌ Unconfirmed uploads become orphans in the bucket. A cleanup job comes with Phase 4's background work.
- ❌ Every picture view costs a little API CPU (streaming from storage). Fine at this scale; a CDN
  (Phase 4.3) absorbs it with the immutable caching already in place.
- ❌ Profile changes aren't pushed live; other users see a new picture on their next fetch.
