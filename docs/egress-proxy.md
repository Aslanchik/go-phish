# Egress Proxy

## Why

The headless browser runs inside Docker and must reach the target URL — but we need to prevent it from reaching anything else (C2 callbacks, data exfiltration endpoints, analytics, CDN resources that obscure the real kit). A plain network allowlist isn't enough because Chromium can bypass per-destination rules via DNS-over-HTTPS or QUIC.

The solution is a per-investigation HTTP CONNECT proxy that runs in the host process, accepts only connections to the target's resolved IPs, and is the container's only egress path.

## Topology

```mermaid
graph TD
    subgraph Host["Host process (cmd/gophish)"]
        Proxy["egressProxy\ninternal/fetcher/proxy.go\nHTTP CONNECT\nbinds :0 (random port)"]
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

## Per-investigation lifecycle

```mermaid
sequenceDiagram
    participant CLI as cmd/gophish
    participant DNS as net.LookupHost
    participant Docker as Docker daemon
    participant Proxy as egressProxy
    participant Container as fetcher container
    participant Target as Target host

    CLI->>DNS: resolve target hostname
    DNS-->>CLI: []string{ip1, ip2, ...}
    CLI->>Docker: docker network create gophish-<nanos>
    CLI->>Proxy: startProxy(allowedIPs) → binds :0
    CLI->>Docker: docker run --network gophish-<nanos> --env HTTP_PROXY=http://host.docker.internal:<port>
    Container->>Proxy: CONNECT target:443
    Proxy->>Proxy: hostAllowed(target:443)?
    Proxy->>Target: TCP tunnel (if allowed)
    Target-->>Container: TLS + HTTP response
    Container-->>CLI: JSON on stdout
    CLI->>Proxy: proxy.stop()
    CLI->>Docker: docker network rm gophish-<nanos>
```

## Proxy bypass vectors closed

| Vector | Mitigation |
|---|---|
| DNS-over-HTTPS (bypasses system resolver) | `--disable-dns-over-https` flag passed to Chromium |
| QUIC (UDP, bypasses TCP proxy) | `--disable-quic` flag passed to Chromium |
| Direct TCP to non-target IPs | HTTP CONNECT handler returns 403 for IPs not in the allow set |
| Container reaching host network | Isolated bridge network; only `host.docker.internal` is reachable |

## Linux vs macOS

On macOS/Windows, Docker Desktop runs inside a Linux VM and injects `host.docker.internal` automatically — the container can always reach the host via this hostname.

On Linux, Docker does not inject `host.docker.internal`. We pass `--add-host=host.docker.internal:host-gateway` to `docker run` so the container resolves it to the host's gateway IP on the bridge.

```go
if runtime.GOOS == "linux" {
    args = append(args, "--add-host=host.docker.internal:host-gateway")
}
```
