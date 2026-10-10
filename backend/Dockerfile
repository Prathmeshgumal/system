# Multi-stage build — see knowledge-base/Notes.md, note 3.

# ---- stage 1: build (big image with the Go compiler) ----
# --platform=$BUILDPLATFORM: compile on your machine's own CPU and cross-compile
# for the target (e.g. an amd64 server from an arm64 Mac) — much faster than
# running the whole Go toolchain under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
WORKDIR /src

# Dependencies first: this layer stays cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG TARGETOS TARGETARCH
# CGO_ENABLED=0 → fully static binary, no dependency on the system C library.
# -s -w strips debug symbols to make the binary smaller.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api

# ---- stage 2: run (tiny image: CA certs, tzdata, nonroot user — nothing else) ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
