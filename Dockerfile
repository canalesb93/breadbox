# Stage 1: Build
FROM golang:1.24-alpine AS builder

ARG VERSION=dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
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
