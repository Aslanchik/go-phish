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

**Decision:** host-side iptables rules scoped to the container, with pre-resolved IPs.

Before starting the container, the Go orchestrator resolves the target domain to its IP addresses. It then runs the container on a dedicated Docker bridge network and installs iptables OUTPUT rules on that network interface that whitelist only those IPs (plus the Docker DNS resolver). All other egress from the container is dropped.

Alternatives considered:
- **`--network none`**: blocks all traffic, so Chromium can't load the page. Not viable.
- **HTTP proxy (e.g. tinyproxy) on the host**: container routes through a proxy that enforces domain allowlist. Clean and auditable, but adds a dependency and a running process per investigation. Revisit if iptables proves fragile.
- **Docker network with `--internal`**: blocks all external egress, same problem as `--network none`.

Known limitation: if the target page loads resources from additional domains (CDN, tracking pixels, etc.), those requests will fail silently inside the browser. This is acceptable for v1 — the primary page renders, and we're investigating the phishing kit itself, not its third-party dependencies.

### Container security posture

```
--cap-drop ALL
--security-opt no-new-privileges
--read-only (with tmpfs for /tmp, required by Chromium)
--network <per-investigation bridge>
```

No host mounts. No privileged mode.

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

The last case is a known limitation. Document it in `agent-notes.md` once the pipeline is running against real phishes, not before.

---

## Open decisions resolved here

**Structured vs. free-form LLM output:** structured via tool_use (see Phase 2 section).

**Pages that fail to render / detect headless:** fail fast with a descriptive error. No fallback fetch, no retry. In v1, a failed fetch is a failed investigation. The failure mode of "cloaked content served" is undetectable and noted as a known gap.

**Phase 3 iteration cap, bad tool ordering:** not applicable to core-pipeline; deferred to the enrichment-tools spec.

---

## Out of scope

Anything not listed above. If it's not in requirements.md, it's not in this design.
