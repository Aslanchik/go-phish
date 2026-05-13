# ---- Stage 1: build React frontend ----
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: build Go server ----
FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Bring the compiled frontend in so //go:embed dist resolves at build time.
COPY --from=frontend /app/web/dist ./web/dist
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /server ./cmd/server

# ---- Stage 3: runtime ----
FROM debian:bookworm-slim
# docker CLI is required: the server spawns go-phish-fetcher containers via exec("docker", ...).
# TODO: when the server itself runs in a container the egress proxy is unreachable via
# host.docker.internal (proxy runs inside this container, not on the host). Fix in run.go
# before enabling the fetch pipeline in production.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        docker.io \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
