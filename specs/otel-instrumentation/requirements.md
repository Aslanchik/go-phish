# otel-instrumentation: Requirements

## Overview

Every go-phish investigation produces a structured OpenTelemetry trace: one root span per investigation, child spans per phase, and nested spans per LLM call and tool invocation. Attributes follow the OTel GenAI semantic conventions where applicable; go-phish-specific concepts that have no standard equivalent live under a documented `ssspy.*` extension namespace. Traces are exported via OTLP HTTP to a configurable endpoint; a file-based fallback exporter allows development and testing without a running collector. A `CONTRACT.md` at the repo root documents the emitted schema so that future consumers (initially ssspy) can build against it without reading go-phish source.

Scope: traces only. No metrics, no logs in this pass.

---

## OT-1: Span hierarchy

Every investigation produces a tree of spans with a well-defined parent–child structure.

**Acceptance criteria:**
- One root span is created per investigation; all other spans are descendants of it
- One child span is created for each pipeline phase: `fetch`, `hypothesis`, `enrichment`, `synthesis`
- Phase spans that involve LLM calls have one child span per LLM call (a single phase may contain multiple LLM calls — each gets its own span)
- Phase 3 (enrichment) has one child span per tool invocation, nested under the enrichment phase span; if a tool is called more than once in a single enrichment loop iteration it still gets its own span per call
- Span parent–child relationships are established via Go context, not by explicitly wiring span IDs in application code
- Spans that have not finished when the investigation ends (due to error or timeout) are ended with an error status before the process exits

---

## OT-2: LLM call spans

Each LLM call span captures the request and response attributes defined by the OTel GenAI semantic conventions.

**Acceptance criteria:**
- Span name follows the semconv pattern: `{gen_ai.operation.name} {gen_ai.request.model}` (e.g., `chat claude-sonnet-4-6`)
- The following attributes are **required** on every LLM call span (span MUST NOT be emitted without them):
  - `gen_ai.operation.name` — `"chat"` for all completions in this codebase
  - `gen_ai.provider.name` — `"anthropic"`
  - `gen_ai.request.model` — the model string passed to the API call
- The following attributes are **required when available** (emitted if the API response includes the field):
  - `gen_ai.response.model` — model string from the response (may differ from request)
  - `gen_ai.response.id` — response ID from the API
  - `gen_ai.response.finish_reasons` — array of finish reason strings
  - `gen_ai.usage.input_tokens` — aggregate input token count (see note below)
  - `gen_ai.usage.output_tokens` — output token count
  - `gen_ai.usage.cache_creation.input_tokens` — tokens written to the prompt cache
  - `gen_ai.usage.cache_read.input_tokens` — tokens read from the prompt cache
- **Token accounting note:** Anthropic reports cache tokens separately from input tokens. `gen_ai.usage.input_tokens` MUST equal `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` as returned by the API, so that the semconv aggregate meaning is preserved. The three cache-specific fields are emitted alongside it for completeness.
- The following attributes are **optional** (emitted when the value was explicitly set in the request):
  - `gen_ai.request.max_tokens`
  - `gen_ai.request.temperature`
  - `gen_ai.request.top_p`
  - `gen_ai.request.top_k`
  - `gen_ai.request.stream`
- If the LLM call fails, the span status is set to `Error` with the error message; the span is still ended
- Span semconv version pinned in `design.md`; the exact version is recorded in `CONTRACT.md`

---

## OT-3: Tool call spans

Each invocation of a Phase 3 enrichment tool produces its own span.

**Acceptance criteria:**
- Span name follows the semconv pattern: `execute_tool {gen_ai.tool.name}` (e.g., `execute_tool whois_lookup`)
- The following attribute is **required**:
  - `gen_ai.tool.name` — the tool name string exactly as used in the tool registry
- The following attributes are **required when available**:
  - `gen_ai.tool.call.id` — if the LLM produces a tool call ID in the request, that ID is carried here
  - `ssspy.tool.input` — JSON-encoded tool input arguments; subject to the payload size policy (OT-6)
  - `ssspy.tool.output` — JSON-encoded tool return value; subject to the payload size policy (OT-6)
- If the tool call returns an error, the span status is set to `Error` with the error message; the span is still ended
- Tool spans are children of the enrichment phase span, not of any individual LLM call span

---

## OT-4: Phase spans

Each pipeline phase has its own span capturing phase-level metadata.

**Acceptance criteria:**
- Span name: `ssspy.phase.{phase}` (e.g., `ssspy.phase.fetch`, `ssspy.phase.hypothesis`)
- The following attributes are **required** on every phase span:
  - `ssspy.investigation.phase` — one of `fetch | hypothesis | enrichment | synthesis`
  - `ssspy.investigation.phase_index` — integer reflecting phase order: `1` for fetch, `2` for hypothesis, `3` for enrichment, `4` for synthesis
- The following attribute is **required when available**:
  - `ssspy.investigation.outcome` — JSON-encoded structured output of the phase (e.g., the hypothesis struct for Phase 2, the synthesis struct for Phase 4); subject to the payload size policy (OT-6)
- If a phase fails, the phase span status is set to `Error` with the error; child spans (LLM calls, tool calls) that were already in flight are also ended with an error status before the phase span closes

---

## OT-5: Investigation root span

One root span per investigation captures investigation-level identity and outcome.

**Acceptance criteria:**
- Span name: `ssspy.investigation`
- The following attributes are **required** on the root span:
  - `ssspy.investigation.id` — the UUID of the investigation, matching the Postgres `investigations.id` column
  - `ssspy.investigation.target_url` — the URL submitted for investigation, normalized (trailing slashes stripped, scheme lowercased)
  - `ssspy.agent.name` — `"go-phish"`
  - `ssspy.agent.version` — the binary version string; defined in `design.md` (git SHA or semver tag)
- If the investigation completes successfully, the root span status is `Ok`
- If the investigation fails at any phase, the root span status is `Error` with a message identifying which phase failed
- The root span is ended after all phase spans are ended and the OTLP exporter flush has been requested

---

## OT-6: Payload size policy

LLM prompts, responses, and tool outputs can be large. OTel collectors have attribute size limits. A consistent policy governs how large payloads are handled.

**Acceptance criteria:**
- A single inline threshold applies to all string attributes that may carry large payloads (`ssspy.tool.input`, `ssspy.tool.output`, `ssspy.investigation.outcome`, and any future opt-in content attributes): **32 KB**
- Payloads at or below 32 KB are emitted inline as string attributes
- Payloads exceeding 32 KB are **truncated** to 32 KB at a valid UTF-8 boundary; a companion boolean attribute `{attribute_name}.truncated` is set to `true` on the same span (e.g., `ssspy.tool.output.truncated: true`)
- Multimodal content (the screenshots passed to the Phase 2 LLM call) is **never** inlined. Instead, the following are emitted on the hypothesis LLM call span:
  - `ssspy.screenshot.content_type` — `"image/png"` or as returned by the fetcher
  - `ssspy.screenshot.size_bytes` — byte size of the image
  - `ssspy.screenshot.sha256` — SHA-256 hex digest of the raw image bytes
- No external sidecar store is required in v1; the hash and size alone are sufficient for ssspy to detect whether the same screenshot appears in multiple traces

---

## OT-7: OTLP HTTP export

The primary exporter sends traces to any OTel-compatible collector over OTLP HTTP.

**Acceptance criteria:**
- Exporter: OTLP HTTP (not gRPC)
- Default endpoint: `http://localhost:4318`
- Endpoint is configurable via the standard `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable; go-phish does not define a custom env var for this
- If `OTEL_EXPORTER_OTLP_ENDPOINT` is set to an empty string, the OTLP exporter is disabled and no traces are sent (the fallback file exporter, if configured, still runs)
- On investigation completion (success or failure), the exporter is flushed before the process exits; a flush timeout of 5 seconds applies
- If the flush fails (collector unreachable, timeout), go-phish logs a warning to stderr but exits with the investigation's own exit code — instrumentation failure does not change the process exit code

---

## OT-8: File export fallback

A file-based exporter allows trace inspection during development when no collector is running.

**Acceptance criteria:**
- When the environment variable `OTEL_FILE_EXPORTER_PATH` is set to a non-empty value, spans are written to that file path as newline-delimited JSON (one JSON object per span)
- Both the OTLP exporter and the file exporter can be active simultaneously; they are additive
- The file is opened in append mode; existing content is not overwritten
- If the file cannot be opened or written (permissions error, disk full), go-phish logs a warning to stderr and continues with OTLP-only export; it does not abort the investigation
- The JSON format for file-exported spans is defined in `design.md`

---

## OT-9: Instrumentation resilience

Failures in the instrumentation layer must not affect the pipeline's correctness or exit code.

**Acceptance criteria:**
- If tracer initialization fails (e.g., OTel SDK setup error), go-phish logs a warning to stderr and continues the investigation using a no-op tracer — the investigation result is unaffected
- If a span operation fails at runtime (attribute set, event add, span end), the error is silently dropped — no panic, no log noise
- go-phish's process exit code is determined solely by investigation outcome, never by instrumentation status
- Under `--skip-llm` (stub mode), instrumentation still runs; spans are created and exported but contain stub attribute values rather than real LLM outputs

---

## OT-10: CONTRACT.md

A machine- and human-readable contract document describes the complete span schema emitted by go-phish.

**Acceptance criteria:**
- `CONTRACT.md` exists at the repo root after this feature ships
- It lists every span type go-phish emits, with:
  - Span name
  - Parent span type
  - Every attribute: name, type, required vs. optional, and a one-line description
  - The semconv version the standard attributes are drawn from
- It distinguishes `ssspy.*` extension attributes from standard semconv attributes
- It documents the payload size policy (thresholds, truncation behavior, screenshot handling)
- It documents the assumption that content is not redacted (personal/local deployment; no PII policy applied)
- It is the authoritative reference for ssspy and future consumers — it must make sense to a reader with no access to go-phish source

---

## Out of scope

- Metrics and logs — traces only in this pass
- Sampling configuration — always-on sampling; no sampler configuration required
- Distributed context propagation — go-phish is a single process
- Performance tuning — instrumentation overhead is not a concern at personal scale
- Automatic redaction of sensitive content (prompts, page content, URLs) — explicitly deferred; documented in `CONTRACT.md`
- Anything ssspy-side — ssspy does not exist yet and is not built here
