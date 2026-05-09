# Architecture

`go-phish` takes a suspicious URL and produces a structured phishing investigation report: brand impersonated, kit mechanics, credential exfiltration destination, and a verdict with per-claim confidence scores.

The pipeline has four phases. Each phase writes its output to Postgres before the next one starts, so a failure mid-run leaves a full audit trail and every investigation is replayable. All four phases are built.

## Pipeline

```mermaid
flowchart TD
    Input(["gophish &lt;url&gt;"])

    subgraph P1["Phase 1 — Fetch"]
        direction TB
        Container["Chromium · Rod\n(Docker container)"]
        Proxy["Egress Proxy\nHTTP CONNECT · IP allowlist"]
    end

    subgraph P2["Phase 2 — Hypothesis"]
        LLM["Claude Sonnet\ntool_use: record_hypothesis"]
    end

    subgraph P3["Phase 3 — Enrichment"]
        AgentLoop["Agent loop\ninternal/agent"]
        MCPServer["MCP Tool Server\ninternal/tools\nwhois · crt.sh · urlscan · urlhaus · analyze_js"]
    end

    subgraph P4["Phase 4 — Synthesis"]
        Synthesis["Claude Sonnet\ntool_use: record_synthesis\nper-claim confidence"]
    end

    DB[("PostgreSQL")]
    Target(["Target host"])
    Report(["stdout"])

    Input -->|"create investigation (pending)"| DB
    Input --> P1
    Container <--> Proxy
    Proxy -->|"allowlisted IPs only"| Target
    P1 -->|"DOM · screenshot · network log · JS · forms"| DB
    DB --> P2
    P2 -->|"brand · action · confidence · reasoning"| DB
    DB --> AgentLoop
    AgentLoop <--> MCPServer
    AgentLoop -->|"enrichment_trace · summary"| DB
    DB --> P4
    P4 -->|"synthesis · report"| DB
    P4 --> Report
```

### Phase 1 — Fetch

The target URL is loaded inside a sandboxed Docker container running Chromium via [Rod](https://go-rod.github.io). An in-process HTTP CONNECT proxy (`internal/fetcher/proxy.go`) runs on the host and is the container's only egress path; it allows connections only to the IPs resolved for the target hostname at startup. QUIC and DNS-over-HTTPS are disabled in Chromium to prevent proxy bypass.

Captured artifacts: rendered DOM, full-page screenshot, network request log, all JS files, all forms with action URLs, and the final URL after any redirect chain.

See [`docs/egress-proxy.md`](egress-proxy.md) for full proxy topology and bypass mitigations.

### Phase 2 — Hypothesis

The screenshot and a structured DOM summary are sent to Claude Sonnet via the Anthropic API using `tool_use`. The model is forced to call `record_hypothesis`, which returns a structured object: `brand`, `targeted_action` (`credential_theft` | `payment_capture` | `mfa_bypass` | `other`), `confidence` (`low` | `medium` | `high`), and `reasoning`.

This is a single-turn call — no agent loop yet. The result is stored in the `hypothesis` JSONB column.

### Phase 3 — Enrichment

An agentic loop (`internal/agent`) gives the model access to an in-process MCP tool server (`internal/tools`) and lets it decide what to investigate and in what order. The loop runs until the model responds with no tool calls or an iteration cap is reached (default 10, configurable via `ENRICHMENT_MAX_TURNS`). Every tool call and result is recorded in `enrichment_trace` and persisted before Phase 4 begins.

Tools: `whois_lookup`, `cert_transparency`, `urlscan_lookup`, `urlhaus_check`, `analyze_js`. The MCP server uses an in-process transport — no network socket, same binary.

### Phase 4 — Synthesis

A single LLM call receives the Phase 2 hypothesis, the full Phase 3 enrichment trace, and a Phase 1 artifact summary (final URL, form actions, JS file count — no raw DOM or screenshot). The model is forced to call `record_synthesis`, which returns five independently assessed claims: `brand_impersonated`, `kit_identification`, `exfil_target`, `infrastructure_notes`, and `verdict`. Each claim carries a `confidence` level (`low | medium | high`) and an `evidence` string that must cite a specific tool output or artifact observation by name. Vague reasoning without a source is rejected by the schema.

Per-claim confidences are the primary anti-hallucination mechanism: when the model says "high confidence," the eval harness checks whether it was right. A global confidence score is not produced.

## Package structure

```mermaid
graph LR
    cmd["cmd/gophish\nCLI · pipeline wiring"]

    subgraph internal
        fetcher["fetcher\nDocker orchestrator\negress proxy"]
        hypothesis["hypothesis\nDOM summary · LLM call"]
        db["db\nmigrations · CRUD"]
        report["report\nplain-text formatter"]
        agent["agent\nagent loop · MCP dispatch"]
        tools["tools\nMCP server · tool handlers"]
        synthesis["synthesis\nLLM call · per-claim verdict"]
    end

    subgraph docker["docker/fetcher"]
        fetcherbin["Rod binary\nDockerfile"]
    end

    cmd --> fetcher
    cmd --> hypothesis
    cmd --> db
    cmd --> report
    cmd --> agent
    cmd --> tools
    cmd --> synthesis
    agent --> tools
    synthesis --> db
    fetcher --> fetcherbin
```

## Status

| Phase | Description | Status |
|---|---|---|
| 1 | Containerised fetch with egress restriction | ✅ Built |
| 2 | LLM hypothesis via `record_hypothesis` tool | ✅ Built |
| 3 | Enrichment agent loop (MCP tool server) | ✅ Built |
| 4 | Synthesis with per-claim confidence | ✅ Built |
