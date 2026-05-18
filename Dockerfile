# Stage 1: Build the v2 SPA bundle. Pinned bun image, runs natively on
# BUILDPLATFORM (not under QEMU). Output (web/dist/) is copied into the Go
# builder below.
FROM --platform=$BUILDPLATFORM oven/bun:1 AS web-builder
WORKDIR /web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

# Stage 1b: Compile the Claude Agent SDK sidecar to a standalone binary.
# Bun cross-compiles to $TARGETARCH from $BUILDPLATFORM, so this runs
# natively (no QEMU). We pick the musl variant because the runtime stage
# below is Alpine (musl libc) — the glibc-targeted Bun binary would not
# load there without gcompat shims.
FROM --platform=$BUILDPLATFORM oven/bun:1 AS sidecar-builder
ARG TARGETARCH
WORKDIR /sidecar
COPY agent/sidecar/package.json agent/sidecar/bun.lock ./
RUN bun install --frozen-lockfile
COPY agent/sidecar/ ./
RUN case "$TARGETARCH" in \
        amd64) TARGET=bun-linux-x64-musl   ;; \
        arm64) TARGET=bun-linux-arm64-musl ;; \
        *) echo "unsupported TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
    esac \
    && bun build --compile --minify --target=$TARGET \
        --outfile /breadbox-agent index.ts

# Stage 2: Build Go binary (runs natively on the build host, cross-compiles Go)
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Replace the committed dist/.gitkeep stub with the real SPA bundle from the
# web-builder stage so //go:embed all:dist picks up the real files.
COPY --from=web-builder /web/dist/ ./web/dist/

# Generate sqlc code (generated files are gitignored)
# Use pre-built binary — compiling from source is very slow under QEMU emulation.
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') \
    && wget -qO- "https://github.com/sqlc-dev/sqlc/releases/download/v1.30.0/sqlc_1.30.0_linux_${ARCH}.tar.gz" | tar xz -C /usr/local/bin sqlc \
    && sqlc generate

# Generate templ components (*.templ → *_templ.go, also gitignored).
# templ's own binary is small and the go install is fast on the build host.
RUN go install github.com/a-h/templ/cmd/templ@latest \
    && /go/bin/templ generate

# Build CSS: download tailwindcss-extra (musl variant for Alpine) and compile input.css
RUN apk add --no-cache libstdc++ libgcc \
    && ARCH=$(uname -m | sed 's/aarch64/arm64/' | sed 's/x86_64/x64/') \
    && wget -qO tailwindcss-extra "https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-linux-${ARCH}-musl" \
    && chmod +x tailwindcss-extra \
    && ./tailwindcss-extra -i input.css -o static/css/styles.css --minify

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /breadbox ./cmd/breadbox

# Stage 3: Runtime
FROM alpine:3.21

# CA certificates: required for TLS connections to Plaid API and PostgreSQL
# tzdata: required for cron schedule timezone handling
# postgresql16-client: required by the /backups page for pg_dump and psql.
#   Must match the server major version (postgres:16-alpine in compose) —
#   pg_dump refuses to dump a newer server.
# libstdc++, libgcc: required by the Bun-compiled breadbox-agent sidecar.
#   Even with --target=bun-linux-*-musl the resulting binary dynamically
#   links libstdc++.so.6 and libgcc_s.so.1, which aren't in alpine base.
#   Without these, the sidecar exits 127 and every agent run fails.
RUN apk --no-cache add ca-certificates tzdata postgresql16-client libstdc++ libgcc

WORKDIR /app
COPY --from=builder /breadbox /app/breadbox
# Sidecar lands on PATH so internal/agent.LocateBinary picks it up via the
# `breadbox-agent` lookup without any extra config.
COPY --from=sidecar-builder /breadbox-agent /usr/local/bin/breadbox-agent

EXPOSE 8080
ENTRYPOINT []
CMD ["/app/breadbox", "serve"]
