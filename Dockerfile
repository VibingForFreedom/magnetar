# ── Build stage ───────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev nodejs npm

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN cd frontend && npm ci && npm run build
RUN rm -rf internal/web/static/_app && cp -r frontend/build/* internal/web/static/
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -o /magnetar ./cmd/magnetar

# ── Runtime stage ─────────────────────────────────────────
FROM ghcr.io/hotio/base:alpine

COPY --from=builder /magnetar /app/magnetar
COPY --from=builder /src/docker/root/ /

RUN mkdir -p /data

ENV MAGNETAR_DB_PATH=/data/magnetar.db

EXPOSE 3333
EXPOSE 6881/udp

VOLUME ["/data"]
