# Egress Proxy

## Why

The headless browser runs inside Docker and must reach the target URL — but we need to prevent it from reaching anything else (C2 callbacks, data exfiltration endpoints, analytics, CDN resources that obscure the real kit). A plain network allowlist isn't enough because Chromium can bypass per-destination rules via DNS-over-HTTPS or QUIC.

The solution is a per-investigation HTTP CONNECT proxy that runs in the server process, accepts only connections to the target's resolved IPs, and is the container's only egress path.

## Topology

Two deployment modes are supported. The proxy logic is identical in both; only the network path from the fetcher container to the proxy changes.

### Local development (server runs on host)

```mermaid
graph TD
    subgraph Host["Host process (go run ./cmd/server)"]
        Proxy["egressProxy\ninternal/fetcher/proxy.go\nHTTP CONNECT\nbinds 0.0.0.0:0 (random port)"]
        AllowList["allowedIPs\nresolved once at startup\nset[string]"]
    end

    subgraph Container["Docker container (go-phish-fetcher)"]
        Chrome["Chromium\n--proxy-server=http://host.docker.internal:<port>\n--disable-quic\n--disable-dns-over-https"]
        Rod["Rod binary\ndocker/fetcher/main.go"]
    end

    subgraph Network["Isolated bridge network (gophish-<nanos>)"]
    end

    Internet["Target URL only"]
    Blocked["All other IPs\n(blocked)"]

    Rod --> Chrome
    Chrome -->|"CONNECT host:443"| Network
    Network -->|"host.docker.internal:<port>"| Proxy
    Proxy -->|"IP in allowedIPs?"| AllowList
    AllowList -->|"yes"| Internet
    AllowList -->|"no → 403"| Blocked
```

### Containerised (docker compose up)

```mermaid
graph TD
    subgraph ServerContainer["Server container (go-phish-server)"]
        Proxy["egressProxy\nbinds 0.0.0.0:0 (random port)"]
        AllowList["allowedIPs"]
    end

    subgraph FetcherContainer["Fetcher container (go-phish-fetcher)"]
        Chrome["Chromium\n--proxy-server=http://server:<port>"]
        Rod["Rod binary"]
    end

    subgraph GophishNet["gophish-net (shared named bridge)"]
    end

    Internet["Target URL only"]
    Blocked["All other IPs\n(blocked)"]

    Rod --> Chrome
    Chrome -->|"CONNECT host:443"| GophishNet
    GophishNet -->|"Docker DNS: server:<port>"| Proxy
    Proxy -->|"IP in allowedIPs?"| AllowList
    AllowList -->|"yes"| Internet
    AllowList -->|"no → 403"| Blocked
```

The fetcher container is attached to `gophish-net` (set via `FETCHER_NETWORK=gophish-net`). The proxy is reachable at `server:<port>` via Docker DNS (set via `PROXY_HOST=server`). Postgres is on a separate network and is unreachable from fetcher containers.

## Per-investigation lifecycle

### Local development

```mermaid
sequenceDiagram
    participant CLI as cmd/server (host)
    participant DNS as net.LookupHost
    participant Docker as Docker daemon
    participant Proxy as egressProxy
    participant Container as fetcher container
    participant Target as Target host

    CLI->>DNS: resolve target hostname
    DNS-->>CLI: []string{ip1, ip2, ...}
    CLI->>Proxy: startProxy(allowedIPs) → binds :0
    CLI->>Docker: docker network create gophish-<nanos>
    CLI->>Docker: docker run --network gophish-<nanos> --env HTTP_PROXY=http://host.docker.internal:<port>
    Container->>Proxy: CONNECT target:443
    Proxy->>Proxy: hostAllowed(target:443)?
    Proxy->>Target: TCP tunnel (if allowed)
    Target-->>Container: TLS + HTTP response
    Container-->>CLI: JSON on stdout
    CLI->>Proxy: proxy.stop()
    CLI->>Docker: docker network rm gophish-<nanos>
```

### Containerised

```mermaid
sequenceDiagram
    participant SRV as cmd/server (container)
    participant DNS as net.LookupHost
    participant Docker as Docker daemon
    participant Proxy as egressProxy
    participant Container as fetcher container
    participant Target as Target host

    SRV->>DNS: resolve target hostname
    DNS-->>SRV: []string{ip1, ip2, ...}
    SRV->>Proxy: startProxy(allowedIPs) → binds :0
    SRV->>Docker: docker run --network gophish-net --env HTTP_PROXY=http://server:<port>
    Container->>Proxy: CONNECT target:443 (via gophish-net → Docker DNS)
    Proxy->>Proxy: hostAllowed(target:443)?
    Proxy->>Target: TCP tunnel (if allowed)
    Target-->>Container: TLS + HTTP response
    Container-->>SRV: JSON on stdout
    SRV->>Proxy: proxy.stop()
```

## Proxy bypass vectors closed

| Vector | Mitigation |
|---|---|
| DNS-over-HTTPS (bypasses system resolver) | `--disable-dns-over-https` flag passed to Chromium |
| QUIC (UDP, bypasses TCP proxy) | `--disable-quic` flag passed to Chromium |
| Direct TCP to non-target IPs | HTTP CONNECT handler returns 403 for IPs not in the allow set |
| Container reaching host network | Isolated bridge network; only `host.docker.internal` is reachable |

## Linux vs macOS (local development only)

When the server runs on the host, the proxy address passed to fetcher containers depends on the OS:

| Platform | `host.docker.internal` | `--add-host` needed? |
|---|---|---|
| macOS / Windows | Injected automatically by Docker Desktop | No |
| Linux | Not injected | Yes — `--add-host=host.docker.internal:host-gateway` |

In containerised mode (`FETCHER_NETWORK` is set) neither applies — Docker DNS resolves the service name directly and `--add-host` is skipped.

```go
// internal/fetcher/run.go
if fetcherNetwork == "" && runtime.GOOS == "linux" {
    args = append(args, "--add-host=host.docker.internal:host-gateway")
}
```

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PROXY_HOST` | `host.docker.internal` | Hostname fetcher containers use to reach the egress proxy |
| `FETCHER_NETWORK` | _(empty)_ | Pre-existing Docker network to attach fetchers to; when set, per-investigation networks are skipped |

Both are set automatically by `docker-compose.yml` when running with `docker compose up`.
