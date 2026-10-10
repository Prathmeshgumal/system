# Multi-stage build — see knowledge-base/Notes.md, note 3.

# ---- stage 1: build the React app into static files ----
# The build output is plain HTML/JS/CSS — the same for every CPU — so build on
# your machine's own platform even when the image is for another one.
FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /app
RUN npm install -g pnpm@12.6.0

# Dependencies first so this layer is cached until the lockfile changes.
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

# ---- stage 2: Caddy serves the files and reverse-proxies /api to the Go API ----
FROM caddy:2-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
