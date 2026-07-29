# ════════════════════════════════════════════════════════════════
# zai2api — Unified AI Gateway
# Multi-stage build for lightweight and secure binary
# ════════════════════════════════════════════════════════════════

# ── Stage 1: Build ────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy go.mod files first for better dependency caching
COPY go.mod go.sum ./
COPY glm/go.mod ./glm/
COPY mimo/go.mod ./mimo/

RUN go mod download || true

# Copy full source (submodules must be present before build)
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/zai2api ./cmd/server

# ── Stage 2: Final (minimal) ──────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --from=builder /out/zai2api /app/zai2api
COPY templates/ /app/templates/
COPY static/ /app/static/

RUN mkdir -p /app/data && chown -R app:app /app

USER app

ENV PORT=8080 \
    GIN_MODE=release \
    ACCOUNTS_DB=/app/data/accounts.sqlite \
    GLM_CAPTCHA_DB=/app/data/tokens.sqlite

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:${PORT}/health || exit 1

ENTRYPOINT ["/app/zai2api"]
