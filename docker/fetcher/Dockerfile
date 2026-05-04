# ---- build stage ----
FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /fetcher .

# ---- runtime stage ----
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        chromium \
        fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /fetcher /fetcher

ENV CHROMIUM_BIN=/usr/bin/chromium

# Runtime security flags — apply these at docker run, not here:
#   --cap-drop ALL --security-opt no-new-privileges
#   --read-only --tmpfs /tmp:size=256m

ENTRYPOINT ["/fetcher"]
