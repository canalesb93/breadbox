# Stage 1: Build (runs natively on the build host, cross-compiles Go)
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG VERSION=dev
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate sqlc code (generated files are gitignored)
# Use pre-built binary — compiling from source is very slow under QEMU emulation.
RUN ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') \
    && wget -qO- "https://github.com/sqlc-dev/sqlc/releases/download/v1.30.0/sqlc_1.30.0_linux_${ARCH}.tar.gz" | tar xz -C /usr/local/bin sqlc \
    && sqlc generate

# Generate templ components (generated *_templ.go files are gitignored)
RUN go install github.com/a-h/templ/cmd/templ@latest \
    && templ generate

# Build CSS: download tailwindcss-extra (musl variant for Alpine) and compile input.css
RUN apk add --no-cache libstdc++ libgcc \
    && ARCH=$(uname -m | sed 's/aarch64/arm64/' | sed 's/x86_64/x64/') \
    && wget -qO tailwindcss-extra "https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-linux-${ARCH}-musl" \
    && chmod +x tailwindcss-extra \
    && ./tailwindcss-extra -i input.css -o static/css/styles.css --minify

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /breadbox ./cmd/breadbox

# Stage 2: Runtime
FROM alpine:3.21

# CA certificates: required for TLS connections to Plaid API and PostgreSQL
# tzdata: required for cron schedule timezone handling
# postgresql16-client: required by the /backups page for pg_dump and psql.
#   Must match the server major version (postgres:16-alpine in compose) —
#   pg_dump refuses to dump a newer server.
RUN apk --no-cache add ca-certificates tzdata postgresql16-client

WORKDIR /app
COPY --from=builder /breadbox /app/breadbox

EXPOSE 8080
ENTRYPOINT []
CMD ["/app/breadbox", "serve"]
