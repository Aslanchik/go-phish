# go-phish Telemetry Contract

## Overview

go-phish emits an OpenTelemetry trace for every investigation. Each trace contains one root span per investigation, one child span per pipeline phase, and nested child spans for every LLM API call and tool invocation. Standard attributes follow the **OTel GenAI semantic conventions v1.41.0**; go-phish-specific concepts that have no standard equivalent are placed under the **`ssspy.*` extension namespace** documented here. Traces are exported via OTLP HTTP (default endpoint `http://localhost:4318`) and optionally written to a local file for development inspection. This document is the authoritative reference for consumers of go-phish traces.

Scope: traces only. No metrics or logs are emitted.

---

## Semconv Version

OTel semantic conventions **v1.41.0** (the GenAI attributes used here remain in Development status upstream; this document pins their exact string forms from that version).

---

## Span Types

### 1. `ssspy.investigation` — Root Span

One per investigation. All other spans are descendants of this span.

**Parent:** none (root)

#### ssspy.* extension attributes

| Attribute | Type | Required | Description |
|---|---|---|---|
| `ssspy.investigation.id` | string | required | UUID of the investigation, matching the Postgres `investigations.id` column |
| `ssspy.investigation.target_url` | string | required | URL submitted for investigation, normalised (scheme lowercased, trailing slashes stripped) |
| `ssspy.agent.name` | string | required | Always `"go-phish"` |
| `ssspy.agent.version` | string | required | First 12 hex chars of the git commit SHA embedded at build time; `"dev"` when build info is unavailable |

**Span status:** `Ok` on success; `Error` with a message identifying which phase failed on failure.

---

### 2. `ssspy.phase.{phase}` — Phase Spans

One per pipeline phase, nested under the root investigation span. The `{phase}` token is one of `fetch`, `hypothesis`, `enrichment`, `synthesis`.

**Parent:** `ssspy.investigation`

#### ssspy.* extension attributes

| Attribute | Type | Required | Description |
|---|---|---|---|
| `ssspy.investigation.phase` | string | required | Phase name: `fetch` \| `hypothesis` \| `enrichment` \| `synthesis` |
| `ssspy.investigation.phase_index` | int | required | Phase order: `1` for fetch, `2` for hypothesis, `3` for enrichment, `4` for synthesis |
| `ssspy.investigation.outcome` | string | conditional | JSON-encoded structured output of the phase. Present on `hypothesis` (the hypothesis struct) and `synthesis` (the synthesis result) phases only. Subject to the 32 KB payload size policy. |
| `ssspy.investigation.outcome.truncated` | bool | conditional | `true` when `ssspy.investigation.outcome` was truncated to 32 KB. Absent otherwise. |

**Span status:** `Ok` when the phase succeeds; `Error` with the error message when it fails. On failure, child spans (LLM calls, tool calls) that were in flight are ended with `Error` before the phase span closes.

---

### 3. `chat {model}` — LLM Call Spans

One per Anthropic API call. The `{model}` token is the model string passed to the API (e.g., `chat claude-sonnet-4-6`).

**Parent:** the phase span of the phase that made the call (`ssspy.phase.hypothesis`, `ssspy.phase.enrichment`, or `ssspy.phase.synthesis`).

#### Standard semconv attributes (gen_ai.*)

| Attribute | Type | Required | Description |
|---|---|---|---|
| `gen_ai.operation.name` | string | required | Always `"chat"` |
| `gen_ai.provider.name` | string | required | Always `"anthropic"` |
| `gen_ai.request.model` | string | required | Model string passed to the API request |
| `gen_ai.request.max_tokens` | int | required | `max_tokens` value from the request |
| `gen_ai.response.model` | string | required when available | Model string from the API response (may differ from request) |
| `gen_ai.response.id` | string | required when available | Response ID returned by the API |
| `gen_ai.response.finish_reasons` | string[] | required when available | Finish reason(s) from the response, wrapped in a one-element slice |
| `gen_ai.usage.input_tokens` | int64 | required when available | Aggregate input tokens: `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` |
| `gen_ai.usage.output_tokens` | int64 | required when available | Output token count from the API response |
| `gen_ai.usage.cache_creation.input_tokens` | int64 | required when available | Tokens written to the prompt cache |
| `gen_ai.usage.cache_read.input_tokens` | int64 | required when available | Tokens read from the prompt cache |

**Token accounting note:** `gen_ai.usage.input_tokens` equals the sum of raw input tokens plus both cache token fields, preserving the semconv aggregate semantics. The three cache-specific fields are emitted alongside it for completeness.

#### ssspy.* extension attributes (hypothesis LLM call only)

These attributes are set on the LLM call span produced by the Phase 2 hypothesis step. Raw screenshot bytes are **never** placed in a span attribute.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `ssspy.screenshot.content_type` | string | required | MIME type of the screenshot, e.g. `"image/png"` |
| `ssspy.screenshot.size_bytes` | int64 | required | Byte size of the raw (pre-base64) screenshot image |
| `ssspy.screenshot.sha256` | string | required | SHA-256 hex digest of the raw image bytes |

**Span status:** `Ok` on success; `Error` with the API error message on failure.

---

### 4. `execute_tool {tool_name}` — Tool Call Spans

One per tool invocation in the Phase 3 enrichment loop. The `{tool_name}` token is the tool name from the MCP tool registry (e.g., `execute_tool whois_lookup`).

**Parent:** `ssspy.phase.enrichment` (tool spans are siblings of LLM call spans, not children of them).

#### Standard semconv attributes (gen_ai.*)

| Attribute | Type | Required | Description |
|---|---|---|---|
| `gen_ai.tool.name` | string | required | Tool name exactly as registered in the MCP tool server |
| `gen_ai.tool.call.id` | string | required when available | Tool call ID produced by the LLM in the tool_use content block, when present |

#### ssspy.* extension attributes

| Attribute | Type | Required | Description |
|---|---|---|---|
| `ssspy.tool.input` | string | required when available | JSON-encoded tool input arguments. Subject to 32 KB payload size policy. |
| `ssspy.tool.input.truncated` | bool | conditional | `true` when `ssspy.tool.input` was truncated. Absent otherwise. |
| `ssspy.tool.output` | string | required when available | JSON-encoded tool return value. Subject to 32 KB payload size policy. Absent when the tool call returned an error. |
| `ssspy.tool.output.truncated` | bool | conditional | `true` when `ssspy.tool.output` was truncated. Absent otherwise. |

**Span status:** `Ok` on success; `Error` with the error message when the tool call fails.

---

## Payload Size Policy

Large string attributes (LLM prompts, tool inputs/outputs, phase outcomes) can exceed OTel collector attribute size limits. The following policy applies uniformly to all attributes that may carry large payloads.

### Inline threshold

**32 KB (32,768 bytes).** Payloads at or below 32 KB are emitted inline as-is.

### Truncation

Payloads exceeding 32 KB are truncated to exactly 32 KB at a valid UTF-8 boundary (no partial multi-byte runes). When truncation occurs, a companion boolean attribute `{attribute_name}.truncated` is set to `true` on the same span.

**Example:** if `ssspy.tool.output` exceeds 32 KB, the span carries both `ssspy.tool.output` (truncated string) and `ssspy.tool.output.truncated: true`.

Attributes subject to this policy: `ssspy.tool.input`, `ssspy.tool.output`, `ssspy.investigation.outcome`, and any future content attributes that opt in.

### Screenshot handling

Multimodal content (the Phase 2 screenshot) is **never inlined** into a span attribute. Instead, the hypothesis LLM call span carries three metadata attributes: `ssspy.screenshot.content_type`, `ssspy.screenshot.size_bytes`, and `ssspy.screenshot.sha256`. The SHA-256 digest is sufficient for a consumer to detect whether the same screenshot appears across multiple traces without requiring blob storage.

---

## Content Redaction

**No redaction is applied.** Full prompts, tool outputs, page content, and URLs are emitted as-is. go-phish is a personal/local deployment tool; no PII policy is in scope for v1. Consumers building on top of go-phish traces are responsible for any redaction required by their own data handling policies.

---

## Exporter Configuration

| Environment variable | Effect |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint. Defaults to `http://localhost:4318`. Set to empty string to disable OTLP export entirely. |
| `OTEL_FILE_EXPORTER_PATH` | If set to a non-empty path, spans are also written to that file as newline-delimited JSON (append mode). Both exporters run simultaneously when both are configured. |
