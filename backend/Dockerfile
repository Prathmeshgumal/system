# Multi-stage build — see knowledge-base/notes/phase-0-notes.md, note 3.

# ---- stage 1: build (big image with the Go compiler) ----
FROM golang:1.27 AS build
WORKDIR /src

# Dependencies first: this layer stays cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
# CGO_ENABLED=0 → fully static binary, no dependency on the system C library.
# -s -w strips debug symbols to make the binary smaller.
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api

# ---- stage 2: run (tiny image: CA certs, tzdata, nonroot user — nothing else) ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
