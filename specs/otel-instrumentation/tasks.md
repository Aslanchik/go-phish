# otel-instrumentation: Tasks

Tasks are ordered. Each must be complete and verifiable before the next begins. If a task surfaces a spec problem, stop — update the relevant spec, re-review, then continue.

Status: `[ ]` todo · `[x]` done · `[~]` in progress

---

## T-01: [x] Add OTel dependencies to go.mod

**Satisfies:** OT-7, OT-8

- Run `go get` to add the four packages listed in design.md:
  - `go.opentelemetry.io/otel`
  - `go.opentelemetry.io/otel/sdk/trace`
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
  - `go.opentelemetry.io/otel/exporters/stdout/stdouttrace`
- Confirm `go.mod` and `go.sum` are updated and the repository still builds (`go build ./...`)

**Verified when:** `go build ./...` succeeds and all four packages appear in `go.mod`.

---

## T-02: [x] Create `internal/telemetry/attrs.go`

**Satisfies:** OT-2, OT-3, OT-4, OT-5

- Create the file `internal/telemetry/attrs.go` with the `telemetry` package declaration
- Declare all `const` string constants for the `gen_ai.*` semconv attributes drawn from OTel semantic conventions v1.41.0, as specified in design.md: `AttrGenAIOperationName`, `AttrGenAIProviderName`, `AttrGenAIRequestModel`, `AttrGenAIRequestMaxTokens`, `AttrGenAIRequestTemperature`, `AttrGenAIRequestTopP`, `AttrGenAIRequestTopK`, `AttrGenAIRequestStream`, `AttrGenAIResponseModel`, `AttrGenAIResponseID`, `AttrGenAIResponseFinishReasons`, `AttrGenAIUsageInputTokens`, `AttrGenAIUsageOutputTokens`, `AttrGenAIUsageCacheCreationInputTokens`, `AttrGenAIUsageCacheReadInputTokens`, `AttrGenAIToolName`, `AttrGenAIToolCallID`
- Declare all `const` string constants for the `ssspy.*` extension namespace as specified in design.md: `AttrInvestigationID`, `AttrTargetURL`, `AttrPhase`, `AttrPhaseIndex`, `AttrPhaseOutcome`, `AttrAgentName`, `AttrAgentVersion`, `AttrToolInput`, `AttrToolOutput`, `AttrScreenshotContentType`, `AttrScreenshotSizeBytes`, `AttrScreenshotSHA256`
- Declare the `TracerName` constant: `"github.com/aslanchik/go-phish"`
- No functions in this file

**Verified when:** `go build ./internal/telemetry/...` succeeds with no errors and no imports required.

---

## T-03: [x] Create `internal/telemetry/payload.go`

**Satisfies:** OT-6

- Create the file `internal/telemetry/payload.go` with the `telemetry` package declaration
- Declare `const PayloadThreshold = 32 * 1024`
- Implement `func Truncate(s string) (value string, truncated bool)`:
  - If `len(s) <= PayloadThreshold`, return `s, false`
  - Otherwise cut at `PayloadThreshold` bytes and walk backwards using `utf8.DecodeLastRuneInString` until the remaining prefix is valid UTF-8; return the truncated prefix and `true`
- Write a unit test in `internal/telemetry/payload_test.go` covering: string below threshold (no truncation), string exactly at threshold (no truncation), ASCII string above threshold (truncated at byte boundary), and a string whose truncation point falls mid-rune (must not produce invalid UTF-8)

**Verified when:** `go test ./internal/telemetry/...` passes with all four cases green.

---

## T-04: [x] Create `internal/telemetry/tracer.go`

**Satisfies:** OT-7, OT-8, OT-9

- Create the file `internal/telemetry/tracer.go` with the `telemetry` package declaration
- Implement `func Init(ctx context.Context) (shutdown func(context.Context) error, err error)` following the steps in design.md exactly:
  1. Check `os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")` — if set to empty string, skip OTLP exporter; if unset or non-empty, create `otlptracehttp.New(ctx)` and wrap in `sdktrace.NewBatchSpanProcessor`
  2. Check `os.Getenv("OTEL_FILE_EXPORTER_PATH")` — if non-empty, open file with `os.O_APPEND|os.O_CREATE|os.O_WRONLY|0o644`; on open failure log warning to stderr and skip; create `stdouttrace` exporter writing to the file handle; wrap in `sdktrace.NewSimpleSpanProcessor`
  3. Build `sdktrace.NewTracerProvider` with `sdktrace.AlwaysSample()` and all active processors
  4. Call `otel.SetTracerProvider(tp)`
  5. Return a shutdown function that calls `tp.Shutdown` and closes the file handle (if open)
  6. On any error in steps 1–4: log warning to stderr, set no-op tracer provider (`trace.NewNoopTracerProvider()`), return no-op shutdown function and the error
- Implement `func Version() string` following the design.md spec: calls `debug.ReadBuildInfo()`, scans `info.Settings` for `vcs.revision`, returns first 12 hex chars; falls back to `"dev"` if build info unavailable, no `vcs.revision` setting, or string shorter than 12 chars (use full string in that case)
- The shutdown function returned by `Init` must respect the context deadline/timeout passed to it

**Verified when:** `go build ./internal/telemetry/...` succeeds; manually running with `OTEL_FILE_EXPORTER_PATH=/tmp/test-spans.jsonl` and a no-op pipeline produces a non-empty file at that path.

---

## T-05: [x] Wire telemetry into `cmd/gophish/main.go`

**Satisfies:** OT-5, OT-7, OT-9

- Read `cmd/gophish/main.go` first
- After `db.CreateInvestigation` returns `inv.ID` and before `pipeline.Run`, add:
  - `telemetry.Init(ctx)` call with warning-log-and-continue on error (OT-9)
  - `defer` shutdown block with a 10-second context timeout
  - Root span `"ssspy.investigation"` started from the global tracer using `otel.Tracer(telemetry.TracerName)` with the four required attributes: `ssspy.investigation.id`, `ssspy.investigation.target_url` (using `normalizeURL`), `ssspy.agent.name` (`"go-phish"`), `ssspy.agent.version` (`telemetry.Version()`)
  - Root span ended with `codes.Ok` on success and `codes.Error` on `pipeline.Run` error
  - Panic-recovery defer that sets error status and ends the root span before re-panicking
- Implement `normalizeURL(raw string) string`: strips trailing slashes, lowercases the scheme portion
- The investigation exit code must be unchanged regardless of telemetry init outcome

**Verified when:** `go build ./cmd/gophish/...` succeeds; running with `OTEL_FILE_EXPORTER_PATH=/tmp/spans.jsonl` produces a span named `ssspy.investigation` in the file.

---

## T-06: [x] Wire telemetry init into `cmd/server/main.go`

**Satisfies:** OT-7, OT-9

- Read `cmd/server/main.go` first
- Add `telemetry.Init(ctx)` call with warning-log-and-continue on error
- Add `defer` shutdown block with a 10-second context timeout
- Do not create any root span at the server level (per design.md — spans are per-request, deferred to future feature)

**Verified when:** `go build ./cmd/server/...` succeeds without errors.

---

## T-07: [x] Add phase spans to `internal/pipeline/pipeline.go`

**Satisfies:** OT-1, OT-4

- Read `internal/pipeline/pipeline.go` first
- Wrap each pipeline phase block in a phase span using `otel.Tracer(telemetry.TracerName)`, shadowing `ctx` per phase so child calls receive the phase span as their parent:
  - Phase 1: span name `"ssspy.phase.fetch"`, attributes `ssspy.investigation.phase="fetch"` and `ssspy.investigation.phase_index=1`; no `AttrPhaseOutcome` (per design.md rationale)
  - Phase 2: span name `"ssspy.phase.hypothesis"`, attributes `ssspy.investigation.phase="hypothesis"` and `ssspy.investigation.phase_index=2`; on success, marshal the hypothesis struct to JSON and attach via `telemetry.Truncate` with `.truncated` companion attribute if needed
  - Phase 3: span name `"ssspy.phase.enrichment"`, attributes `ssspy.investigation.phase="enrichment"` and `ssspy.investigation.phase_index=3`; no `AttrPhaseOutcome` (per design.md rationale)
  - Phase 4: span name `"ssspy.phase.synthesis"`, attributes `ssspy.investigation.phase="synthesis"` and `ssspy.investigation.phase_index=4`; on success, marshal synthesis result and attach with truncation policy
- On phase error: call `span.RecordError(err)`, `span.SetStatus(codes.Error, err.Error())`, `span.End()`, then return
- On phase success: `span.End()` before moving to the next phase
- Pass the phase-scoped context (`fetchCtx`, `hypCtx`, etc.) into the corresponding sub-call

**Verified when:** `go build ./internal/pipeline/...` succeeds; running an investigation with `OTEL_FILE_EXPORTER_PATH=/tmp/spans.jsonl` and `--skip-llm` produces four phase spans in the file, each with correct `ssspy.investigation.phase` and `ssspy.investigation.phase_index` attributes.

---

## T-08: [x] Add LLM call span and screenshot attributes to `internal/hypothesis/generate.go`

**Satisfies:** OT-1, OT-2, OT-6

- Read `internal/hypothesis/generate.go` first
- Add an unexported `screenshotSHA256(b []byte) string` helper that computes `sha256.Sum256(b)` and returns the hex string
- Add an unexported `setLLMResponseAttrs(span trace.Span, resp *anthropic.Message)` helper that sets: `gen_ai.response.model`, `gen_ai.response.id`, `gen_ai.usage.input_tokens` (as the aggregate: `InputTokens + CacheReadInputTokens + CacheCreationInputTokens`), `gen_ai.usage.output_tokens`, `gen_ai.usage.cache_creation.input_tokens`, `gen_ai.usage.cache_read.input_tokens`, and `gen_ai.response.finish_reasons` (wrapped in a one-element string slice from `resp.StopReason`, emitted only when non-empty)
- Before the `client.Messages.New` call, start an LLM call span named `"chat " + string(model)` with the required request attributes and the three screenshot attributes (`ssspy.screenshot.content_type`, `ssspy.screenshot.size_bytes`, `ssspy.screenshot.sha256`) — do not inline the raw screenshot bytes
- After the API call: on error set span status to `Error` and end; on success call `setLLMResponseAttrs` then end
- Span must be created with `otel.Tracer(telemetry.TracerName)` using the hypothesis phase context so it is a child of the hypothesis phase span

**Verified when:** `go build ./internal/hypothesis/...` succeeds; a trace captured via the file exporter shows an LLM call span as a child of `ssspy.phase.hypothesis` with `ssspy.screenshot.sha256` populated and no raw image bytes in any attribute.

---

## T-09: [x] Add LLM call span to `internal/synthesis/synthesis.go`

**Satisfies:** OT-1, OT-2

- Read `internal/synthesis/synthesis.go` first
- Add an unexported `setLLMResponseAttrs(span trace.Span, resp *anthropic.Message)` helper, duplicated from the pattern in design.md (identical logic, local copy — do not import from `hypothesis`)
- Before the `client.Messages.New` call, start an LLM call span named `"chat " + string(model)` with the required request attributes
- After the API call: on error set span status to `Error` and end; on success call `setLLMResponseAttrs` then end
- Span must be a child of the synthesis phase context

**Verified when:** `go build ./internal/synthesis/...` succeeds; a trace from the file exporter shows an LLM call span as a child of `ssspy.phase.synthesis` with `gen_ai.usage.input_tokens` populated.

---

## T-10: [x] Add LLM call spans and tool call spans to `internal/agent/run.go`

**Satisfies:** OT-1, OT-2, OT-3

- Read `internal/agent/run.go` first
- Add an unexported `setLLMResponseAttrs(span trace.Span, resp *anthropic.Message)` helper, duplicated locally (same pattern as T-08 and T-09)
- For each Anthropic API call in the agent turn loop:
  - Start an LLM call span named `"chat " + string(model)` from `ctx` (the enrichment phase context) with the required request attributes
  - Pass `llmCtx` to `anthropicClient.Messages.New` so future HTTP-level OTel propagates correctly
  - On error: set span status to `Error`, end span, return error
  - On success: call `setLLMResponseAttrs(llmSpan, resp)` then end span
- For each tool dispatch in the content block loop, replace the bare `dispatchTool` call with the instrumented pattern from design.md:
  - Start a tool span named `"execute_tool " + tu.Name` from `ctx` (the enrichment phase context — not `llmCtx`)
  - Set `gen_ai.tool.name`; set `gen_ai.tool.call.id` only when `tu.ID != ""`
  - Apply `telemetry.Truncate` to the JSON-encoded input; set `ssspy.tool.input` and companion `.truncated` boolean if needed
  - Call `dispatchTool(toolCtx, ...)` 
  - On error: set span status to `Error`, end span
  - On success: apply `telemetry.Truncate` to the JSON-encoded output; set `ssspy.tool.output` and companion `.truncated` boolean if needed; end span

**Verified when:** `go build ./internal/agent/...` succeeds; a live enrichment run captured via the file exporter shows LLM call spans and tool call spans both as children of `ssspy.phase.enrichment`, with tool spans carrying `gen_ai.tool.name` and `ssspy.tool.input`.

---

## T-11: [x] Write `CONTRACT.md` at the repo root

**Satisfies:** OT-10

- Create `CONTRACT.md` at the repo root (not inside `specs/`)
- The document must include, in this order:
  1. A one-paragraph overview: what go-phish emits, the semconv version (v1.41.0), and the `ssspy.*` extension namespace
  2. A section for each span type emitted — `ssspy.investigation`, `ssspy.phase.{phase}`, `chat {model}` (LLM call), `execute_tool {tool_name}` (tool call) — each listing: span name pattern, parent span type, and a table of every attribute (name, type, required vs. optional/conditional, one-line description)
  3. Clear separation between standard semconv attributes and `ssspy.*` extension attributes within each span section
  4. A payload size policy section: the 32 KB inline threshold, truncation behaviour, the `.truncated` companion attribute convention, and the screenshot handling rule (hash + size, never inline bytes)
  5. A note that content is not redacted (personal/local deployment; no PII policy)
  6. The semconv version pinned: v1.41.0
- The document must make sense to a reader with no access to go-phish source

**Verified when:** The file exists at `/CONTRACT.md`; it contains entries for all four span types; the payload size policy section is present; the semconv version is stated.

---

## T-12: [x] Update `docs/architecture.md` to reflect the new `internal/telemetry` package

**Satisfies:** Working agreement in CLAUDE.md — docs must reflect package structure changes

- Read `docs/architecture.md` first
- Add `internal/telemetry` to the package diagram/description, noting its three files (`tracer.go`, `attrs.go`, `payload.go`) and its role (tracer init, attribute constants, payload truncation)
- Note that span creation is distributed across `cmd/gophish/main.go`, `internal/pipeline`, `internal/hypothesis`, `internal/synthesis`, and `internal/agent` — not centralised in `internal/telemetry`
- Do not change any other content

**Verified when:** `docs/architecture.md` contains a reference to `internal/telemetry` and its three constituent files.

---

## T-13: End-to-end smoke test

**Satisfies:** OT-1, OT-2, OT-3, OT-4, OT-5, OT-7, OT-8, OT-9

- Run a full investigation against a known URL with `OTEL_FILE_EXPORTER_PATH=/tmp/smoke-spans.jsonl` set
- Verify the output file contains spans for: `ssspy.investigation` (root), `ssspy.phase.fetch`, `ssspy.phase.hypothesis`, `ssspy.phase.enrichment`, `ssspy.phase.synthesis`, at least one `chat ...` LLM span, at least one `execute_tool ...` tool span
- Verify parent–child relationships are correct: phase spans are children of the root; LLM and tool spans are children of their respective phase spans
- Verify the investigation exit code is `0` (success) regardless of any exporter warnings
- Run with `OTEL_EXPORTER_OTLP_ENDPOINT=` (empty string) and confirm the OTLP exporter is disabled with no error exit
- Run with `OTEL_FILE_EXPORTER_PATH=/nonexistent/path/spans.jsonl` and confirm a warning is logged to stderr but the investigation still completes successfully

**Verified when:** All six checks above pass in a single terminal session; no untracked binary files remain after the test.
