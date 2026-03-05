# Stage 1: Build
FROM golang:1.24-alpine AS builder

ARG VERSION=dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build CSS: download tailwindcss-extra and compile input.css
RUN curl -sLo tailwindcss-extra https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-linux-x64 \
    && chmod +x tailwindcss-extra \
    && ./tailwindcss-extra -i input.css -o static/css/styles.css --minify

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /breadbox ./cmd/breadbox

# Stage 2: Runtime
FROM alpine:3.21

# CA certificates: required for TLS connections to Plaid API and PostgreSQL
# tzdata: required for cron schedule timezone handling
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /breadbox /app/breadbox

EXPOSE 8080
ENTRYPOINT []
CMD ["/app/breadbox", "serve"]
