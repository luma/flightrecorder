# ── Stage 1: Frontend ────────────────────────────────────────────────────────
FROM oven/bun:1-alpine AS frontend
WORKDIR /build
COPY web-vite/package.json web-vite/bun.lock ./
RUN bun install --frozen-lockfile
COPY web-vite/ ./
RUN bun run build

# ── Stage 2: Go binary ───────────────────────────────────────────────────────
FROM golang:1.25 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Overlay frontend build output into the embed directory, preserving embed.go
COPY --from=frontend /build/dist/ api/spa/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s' \
    -o flightrecorder .

# ── Stage 3: Runtime ─────────────────────────────────────────────────────────
FROM scratch
# CA certs for TLS connections to PlanetScale and R2
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/flightrecorder /app/flightrecorder
EXPOSE 8080
ENTRYPOINT ["/app/flightrecorder"]
CMD ["serve"]
