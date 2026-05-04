# core-pipeline: Design

## Overview

This document specifies how the core pipeline is built: the internal architecture, data models, interface contracts, and the key decisions made with their rationale. Implementation follows this document; prompts do not appear here.

---

## Package structure

```
cmd/gophish/main.go       — CLI parsing, wires phases together, owns the top-level error
internal/fetcher/         — container lifecycle, result parsing
internal/hypothesis/      — Phase 2 LLM call and structured output handling
internal/db/              — Postgres connection, migration runner, typed queries
internal/report/          — formats and prints the investigation report
```

Phase 3 and Phase 4 packages (`enrichment/`, `synthesis/`, `agent/`, `tools/`) are created as empty stubs so they have a home when specced. No logic lives there during core-pipeline.

---

## CLI

Standard library `flag` package. No third-party CLI framework — the surface is one subcommand and one argument; a framework adds nothing.

```
gophish <url>
gophish --help
```

`main.go` wires the pipeline: validate URL → ensure DB → run fetcher → run hypothesis → store → print report. Each step returns a typed error; the first failure exits non-zero.

---

## Phase 1: Fetcher container

### Technology choice: Rod

**Decision:** Rod (`github.com/go-rod/rod`).

Rod manages the Chromium binary automatically (no separate install step), its API is higher-level and more readable than chromedp's raw CDP calls, and for what Phase 1 needs — navigate, wait for load, capture DOM/screenshot/network — Rod's surface is a direct fit. chromedp is more battle-tested and closer to the wire, which matters if we hit browser-control edge cases, but that's a future problem. Rod wins on simplicity for v1.

The fetcher binary (inside the container) is a small Go program in `docker/fetcher/`. It is separate from the main module — its only job is to drive Rod and emit JSON.

### Container interface

**Host → Container:**

| Mechanism | Value |
|---|---|
| Env var `TARGET_URL` | The URL to investigate |
| Env var `FETCH_TIMEOUT_SECONDS` | Page load timeout (default: 30) |

No filesystem mounts. No open ports.

**Container → Host:**

- **stdout:** a single JSON object on success (schema below)
- **stderr:** diagnostic logs, not parsed by the host
- **exit code:** 0 = success, non-zero = failure; host treats any non-zero exit as an investigation failure

**Output JSON schema:**

```json
{
  "final_url":    "string — URL after all redirects",
  "rendered_dom": "string — full HTML of the rendered page",
  "screenshot":   "string — base64-encoded PNG",
  "network_log":  [
    { "url": "string", "method": "string", "status": 0, "resource_type": "string" }
  ],
  "js_files": [
    { "url": "string", "content": "string" }
  ],
  "forms": [
    {
      "action": "string",
      "method": "string",
      "fields": [ { "name": "string", "type": "string" } ]
    }
  ]
}
```

The host unmarshals this into a typed Go struct in `internal/fetcher/`. If stdout is not valid JSON or the container exits non-zero, the investigation fails.

### Egress restriction

**Decision:** host-side HTTP CONNECT proxy with IP allowlist, cross-platform.

Before starting the container, the Go orchestrator:
1. Resolves the target domain to its IP addresses.
2. Starts an in-process HTTP CONNECT proxy (listening on `0.0.0.0:0`) that only forwards connections whose resolved destination IP is in the allowlist.
3. Passes `HTTP_PROXY` and `HTTPS_PROXY` pointing to `http://host.docker.internal:<proxy-port>` to the container.
4. On Linux, adds `--add-host=host.docker.internal:host-gateway` to the `docker run` call so the hostname resolves; Docker Desktop on macOS/Windows injects it automatically.
5. Configures Chromium with `--proxy-server`, `--disable-quic`, and `--disable-dns-over-https` to prevent UDP-based and DoH-based proxy bypass.

Alternatives considered:
- **`--network none`**: blocks all traffic, so Chromium can't load the page. Not viable.
- **Host-side iptables FORWARD rules**: works on Linux only — Docker Desktop on macOS runs containers in a Linux VM so host iptables rules don't reach them. Implemented initially, replaced by proxy for cross-platform parity.
- **Docker network with `--internal`**: blocks all external egress, same problem as `--network none`.
- **Privileged setup container writing iptables into the Docker VM**: rules affect the shared VM network namespace; cleanup failures could break unrelated containers. Too dangerous.
- **Gateway IP from `docker network inspect`**: attempted first; fails on macOS Docker Desktop because the bridge gateway is inside the Linux VM and not reachable from the macOS host where the proxy listens.

The proxy approach is chosen over iptables because it works identically on macOS and Linux, and the connection log it produces is a useful audit trail. `host.docker.internal` is used instead of the bridge gateway IP to correctly traverse the macOS Docker Desktop VM boundary.

Known limitation: if the target page loads resources from additional domains (CDN, tracking pixels, etc.), those requests will be blocked by the proxy. This is acceptable for v1 — the primary page renders, and we're investigating the phishing kit itself, not its third-party dependencies.

### Container security posture

```
--cap-drop ALL
--security-opt no-new-privileges
--read-only (with tmpfs for /tmp, required by Chromium)
--network <per-investigation bridge>
```

No host mounts. No privileged mode.

**Implementation note — Rod `Leakless` disabled (`Leakless(false)`):**

Rod's launcher normally extracts a helper binary (`leakless`) into `/tmp` at startup to reap orphaned browser processes, then executes it. In a `--read-only` container (even with a tmpfs at `/tmp`), Docker Desktop on macOS prevents execution of binaries written to that tmpfs, causing a `permission denied` at launch.

Options considered:

| Option | Pro | Con |
|---|---|---|
| `Leakless(false)` | Simple; leakless unnecessary in Docker since container lifetime = process lifetime | If the process is killed mid-run, Chrome may not be reaped (harmless — container is destroyed anyway) |
| `--tmpfs /tmp:exec,size=256m` | Keeps leakless enabled | Exec behaviour on tmpfs varies across Docker environments; doesn't address the root cause |
| Drop `--read-only` | No exec restriction | Significantly weakens security posture; not acceptable |

**Decision:** `Leakless(false)`. The safety property leakless provides (orphan reaping) is irrelevant inside a container.

---

## Phase 2: Hypothesis generation

### LLM input

Constructed in `internal/hypothesis/`. Two inputs:

1. **Screenshot** — base64 PNG passed as an image content block
2. **DOM summary** — not the raw DOM; a structured extract:
   - `<title>` content
   - `<meta name="description">` content
   - All form fields (names, types, action URLs) — already captured in Phase 1
   - Visible text, truncated to 2000 characters

The DOM summary is generated deterministically in Go (no LLM call). Raw DOM can exceed hundreds of KB and contains little signal for hypothesis generation.

### Structured output via tool_use

**Decision:** use the Anthropic API's tool_use mechanism to enforce structured output. The model is given a single tool, `record_hypothesis`, and instructed to call it. The host reads the tool_use block from the response and unmarshals the arguments — no text parsing.

`record_hypothesis` input schema:

```json
{
  "brand": "string — the brand or organization being impersonated",
  "targeted_action": "credential_theft | payment_capture | mfa_bypass | other",
  "confidence": "low | medium | high",
  "reasoning": "string — one or two sentences explaining the confidence rating"
}
```

`reasoning` is required. It makes confidence auditable in evals and surfaces hallucinations (a model that says "high confidence" with vague reasoning is a signal).

Alternatives considered:
- **Free-form text + parsing**: flexible but fragile; parsing errors are investigation failures; harder to eval
- **`response_format: json_schema`** (if/when available in the Go SDK): cleaner but tool_use achieves the same result today and is idiomatic for the Anthropic SDK

If the model does not call `record_hypothesis` (calls nothing, or returns a text block), the investigation fails with an explicit error. No retry in v1.

---

## Postgres schema

### Technology choice: goose

**Decision:** goose (`github.com/pressly/goose/v3`).

goose supports embedded migrations (the migration SQL files are embedded in the binary via `embed.FS`), so there is no separate migration binary or manual file distribution. Migrations run at application startup if there are unapplied ones. golang-migrate is equally capable, but goose's Go-native embedding story is cleaner for a CLI tool that runs on a single laptop.

Migration files live at `internal/db/migrations/` (not a top-level `migrations/` directory). The `go:embed` directive cannot traverse `../`, so the files must be inside the package tree of the embedding file (`internal/db/`). This is the only structural divergence from the initial directory layout sketch.

### Schema

```sql
-- 0001_create_investigations.sql

CREATE TABLE investigations (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    url             TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    status          TEXT        NOT NULL DEFAULT 'pending',
                    -- pending | fetching | hypothesizing | complete | failed
    error_message   TEXT,

    -- Phase 1 artifacts
    final_url       TEXT,
    rendered_dom    TEXT,
    screenshot      BYTEA,
    network_log     JSONB,
    js_files        JSONB,
    forms           JSONB,

    -- Phase 2 output
    hypothesis      JSONB,
    -- { brand, targeted_action, confidence, reasoning }

    -- Final report
    report          TEXT
);

-- 0002_create_eval_labels.sql

CREATE TABLE eval_labels (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id    UUID        NOT NULL REFERENCES investigations(id),
    labeled_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    labeled_by          TEXT,
    brand_impersonated  TEXT        NOT NULL,
    exfil_destination   TEXT,
    kit_name            TEXT,
    is_actually_phishing BOOLEAN    NOT NULL
);
```

**Artifact storage in Postgres:** screenshots and DOM are stored as BYTEA/TEXT columns, not on disk. For a single-laptop tool investigating tens to hundreds of phishes, this is fine. Avoids a separate storage layer and keeps investigations self-contained. Revisit if the table grows unwieldy.

### Connection

Database URL from environment variable `DATABASE_URL` (`postgres://...`). No config file in v1. The connection is validated at startup; if it fails, the tool exits before doing any work.

---

## Failure modes

| Failure | Behavior |
|---|---|
| DB unreachable at startup | Exit immediately, before fetching |
| Container fails to start | Investigation fails; error stored if DB is available |
| Page load times out | Investigation fails with timeout error |
| Container emits invalid JSON | Investigation fails; raw stderr logged |
| LLM call fails / rate-limited | Investigation fails; no retry in v1 |
| LLM does not call `record_hypothesis` | Investigation fails with explicit protocol error |
| Page serves cloaked content | Not detectable in v1; treated as a successful fetch of whatever was returned |
| Process killed mid-investigation | Row stays in non-terminal status (`fetching`, `hypothesizing`) indefinitely — **known limitation, v1 accepted** |

The cloaked-content case is a known limitation. Document it in `agent-notes.md` once the pipeline is running against real phishes, not before.

The stuck-row case is also accepted for v1. To manually reset: `UPDATE investigations SET status = 'failed', error_message = 'interrupted' WHERE status NOT IN ('complete', 'failed');` The upgrade path (startup scan) is a 5-line fix deferred until it bites us in practice.

---

## Open decisions resolved here

**Structured vs. free-form LLM output:** structured via tool_use (see Phase 2 section).

**Pages that fail to render / detect headless:** fail fast with a descriptive error. No fallback fetch, no retry. In v1, a failed fetch is a failed investigation. The failure mode of "cloaked content served" is undetectable and noted as a known gap.

**Phase 3 iteration cap, bad tool ordering:** not applicable to core-pipeline; deferred to the enrichment-tools spec.

---

## Out of scope

Anything not listed above. If it's not in requirements.md, it's not in this design.
