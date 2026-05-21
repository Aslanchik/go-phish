# otel-instrumentation: Design

## Overview

Instrumentation is threaded through the existing pipeline via Go context. A new `internal/telemetry` package owns tracer initialisation, shutdown, attribute name constants, and the payload truncation helper. Every span is created with `otel.Tracer(...)` — the global tracer — so no function signatures change except that callers must pass the phase-scoped context (rather than the root context) when invoking sub-phases. The OTLP HTTP exporter is the primary sink; a stdouttrace-to-file exporter adds a development fallback. Both run simultaneously when configured.

---

## Semconv version

**Pin: OTel semantic conventions v1.41.0** (latest stable at time of spec writing).

The Go SDK's `go.opentelemetry.io/otel/semconv/vX.Y.Z` package lags the spec for GenAI attributes, which remain in Development status. To avoid depending on an under-specified Go package, all attribute name strings are declared as typed constants in `internal/telemetry/attrs.go`, copied verbatim from the v1.41.0 spec. The exact version is recorded in `CONTRACT.md`. When upgrading, `attrs.go` is the single file to review against the new spec.

---

## Package structure

```
internal/telemetry/
    tracer.go    — Init(), Shutdown(), Version(), tracer name constant
    attrs.go     — all attribute name string constants (gen_ai.*, ssspy.*)
    payload.go   — Truncate() helper and the 32 KB threshold constant
```

No other new packages. All span creation happens in the existing packages that own the relevant operations.

**Tracer name:** `"github.com/aslanchik/go-phish"` — the module path, consistent with OTel instrumentation conventions.

---

## New dependencies

```
go.opentelemetry.io/otel                                        — core API (tracer, span, attribute, codes)
go.opentelemetry.io/otel/sdk/trace                              — TracerProvider, span processors
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp — OTLP HTTP exporter
go.opentelemetry.io/otel/exporters/stdout/stdouttrace           — file fallback exporter
```

Exact versions are recorded in `go.mod` and `go.sum` at implementation time.

---

## internal/telemetry/tracer.go

### Init

```go
func Init(ctx context.Context) (shutdown func(context.Context) error, err error)
```

Steps:
1. Read `os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")`.
   - If the env var is set to an **empty string**: OTLP exporter is disabled.
   - If unset OR non-empty: create the OTLP HTTP exporter. When unset, `otlptracehttp.New()` will apply its own default (`http://localhost:4318`). When set to a non-empty value, the exporter reads it automatically.
2. If the OTLP exporter is enabled, create it with `otlptracehttp.New(ctx)` and wrap it in a `sdktrace.NewBatchSpanProcessor`.
3. Read `os.Getenv("OTEL_FILE_EXPORTER_PATH")`. If non-empty:
   - Open the file with `os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)`.
   - On open failure: log a warning to stderr, skip the file exporter, continue.
   - Create a `stdouttrace` exporter writing to the file handle.
   - Wrap in a `sdktrace.NewSimpleSpanProcessor` (file writes are synchronous; batch is unnecessary for dev-scale).
4. Build a `sdktrace.NewTracerProvider` with `sdktrace.AlwaysSample()` and all active processors.
5. Call `otel.SetTracerProvider(tp)`.
6. Return a shutdown function that calls `tp.Shutdown` followed by closing the file handle (if open). The returned `error` is from `tp.Shutdown`.

On any error in step 1–5: log a warning to stderr, set a no-op tracer provider (`trace.NewNoopTracerProvider()`), return a no-op shutdown function and the error. The caller in `main.go` logs the warning but continues — the investigation is not aborted.

### Version

```go
func Version() string
```

Calls `debug.ReadBuildInfo()` and extracts the `vcs.revision` setting from `BuildInfo.Settings`. Returns the first 12 characters of the hex SHA. Falls back to `"dev"` if build info is unavailable or the setting is absent (common in `go test` and `go run`).

---

## internal/telemetry/attrs.go

All constants are `const` strings. Groupings:

```go
// GenAI semconv — drawn from OTel semantic conventions v1.41.0
const (
    AttrGenAIOperationName                  = "gen_ai.operation.name"
    AttrGenAIProviderName                   = "gen_ai.provider.name"
    AttrGenAIRequestModel                   = "gen_ai.request.model"
    AttrGenAIRequestMaxTokens               = "gen_ai.request.max_tokens"
    AttrGenAIRequestTemperature             = "gen_ai.request.temperature"
    AttrGenAIRequestTopP                    = "gen_ai.request.top_p"
    AttrGenAIRequestTopK                    = "gen_ai.request.top_k"
    AttrGenAIRequestStream                  = "gen_ai.request.stream"
    AttrGenAIResponseModel                  = "gen_ai.response.model"
    AttrGenAIResponseID                     = "gen_ai.response.id"
    AttrGenAIResponseFinishReasons          = "gen_ai.response.finish_reasons"
    AttrGenAIUsageInputTokens               = "gen_ai.usage.input_tokens"
    AttrGenAIUsageOutputTokens              = "gen_ai.usage.output_tokens"
    AttrGenAIUsageCacheCreationInputTokens  = "gen_ai.usage.cache_creation.input_tokens"
    AttrGenAIUsageCacheReadInputTokens      = "gen_ai.usage.cache_read.input_tokens"
    AttrGenAIToolName                       = "gen_ai.tool.name"
    AttrGenAIToolCallID                     = "gen_ai.tool.call.id"
)

// ssspy extension namespace
const (
    AttrInvestigationID        = "ssspy.investigation.id"
    AttrTargetURL              = "ssspy.investigation.target_url"
    AttrPhase                  = "ssspy.investigation.phase"
    AttrPhaseIndex             = "ssspy.investigation.phase_index"
    AttrPhaseOutcome           = "ssspy.investigation.outcome"
    AttrAgentName              = "ssspy.agent.name"
    AttrAgentVersion           = "ssspy.agent.version"
    AttrToolInput              = "ssspy.tool.input"
    AttrToolOutput             = "ssspy.tool.output"
    AttrScreenshotContentType  = "ssspy.screenshot.content_type"
    AttrScreenshotSizeBytes    = "ssspy.screenshot.size_bytes"
    AttrScreenshotSHA256       = "ssspy.screenshot.sha256"
)
```

No functions in this file — constants only.

---

## internal/telemetry/payload.go

```go
const PayloadThreshold = 32 * 1024 // 32 KB

// Truncate cuts s to at most PayloadThreshold bytes at a valid UTF-8 boundary.
// Returns the (possibly truncated) string and whether truncation occurred.
func Truncate(s string) (value string, truncated bool)
```

Implementation: if `len(s) <= PayloadThreshold`, return as-is. Otherwise, cut at `PayloadThreshold` and walk backwards until a valid UTF-8 rune boundary is found (using `utf8.ValidString` or iterating with `utf8.DecodeLastRuneInString`).

Callers use the pair pattern:
```go
v, trunc := telemetry.Truncate(someString)
span.SetAttributes(attribute.String(telemetry.AttrToolOutput, v))
if trunc {
    span.SetAttributes(attribute.Bool(telemetry.AttrToolOutput+".truncated", true))
}
```

---

## Span creation: cmd/gophish/main.go

After `db.CreateInvestigation` returns `inv.ID` and before `pipeline.Run`:

```go
shutdown, initErr := telemetry.Init(ctx)
if initErr != nil {
    log.Printf("warn: telemetry init failed: %v — continuing without traces", initErr)
}
defer func() {
    shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := shutdown(shutCtx); err != nil {
        log.Printf("warn: telemetry shutdown: %v", err)
    }
}()

tracer := otel.Tracer(telemetry.TracerName)
ctx, rootSpan := tracer.Start(ctx, "ssspy.investigation",
    trace.WithAttributes(
        attribute.String(telemetry.AttrInvestigationID, inv.ID),
        attribute.String(telemetry.AttrTargetURL, normalizeURL(rawURL)),
        attribute.String(telemetry.AttrAgentName, "go-phish"),
        attribute.String(telemetry.AttrAgentVersion, telemetry.Version()),
    ))
defer func() {
    if r := recover(); r != nil {
        rootSpan.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
        rootSpan.End()
        panic(r)
    }
}()
```

On success: `rootSpan.SetStatus(codes.Ok, "")` then `rootSpan.End()`.
On `pipeline.Run` error: `rootSpan.SetStatus(codes.Error, err.Error())` then `rootSpan.End()`.

`normalizeURL` strips trailing slashes and lowercases the scheme — reuses the same normalisation already implied by the URL validation logic.

The 10-second shutdown timeout is generous; in practice the batch exporter flush is fast. The 5-second requirement from OT-7 is the flush timeout for the exporter itself; the 10-second wrapper gives headroom for both exporter flush and file handle close.

**Also update `cmd/server/main.go`** with the same `telemetry.Init` / `defer shutdown` wiring so the web server also emits traces. Tracer setup is identical; no root span at the server level (spans are per-request, created inside handler middleware in a future feature). For now, the server just initialises the provider and exits cleanly.

---

## Span creation: internal/pipeline/pipeline.go

The existing `Run` function is reorganised so each phase block is bracketed by a phase span. `ctx` is shadowed per-phase so child calls receive the phase span as parent.

### Phase 1 — fetch

```go
fetchCtx, fetchSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.fetch",
    trace.WithAttributes(
        attribute.String(telemetry.AttrPhase, "fetch"),
        attribute.Int(telemetry.AttrPhaseIndex, 1),
    ))

result, err := fetcher.Run(fetchCtx, inv.URL)
if err != nil {
    fetchSpan.RecordError(err)
    fetchSpan.SetStatus(codes.Error, err.Error())
    fetchSpan.End()
    return fail("fetch: %w", err)
}
fetchSpan.End()
```

No `AttrPhaseOutcome` on the fetch phase — the artifact blob is large and already stored in Postgres. The fetch span records success/failure only.

### Phase 2 — hypothesis

```go
hypCtx, hypSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.hypothesis",
    trace.WithAttributes(
        attribute.String(telemetry.AttrPhase, "hypothesis"),
        attribute.Int(telemetry.AttrPhaseIndex, 2),
    ))

hyp, err = hypothesis.Generate(hypCtx, llmClient, screenshotBytes, result.RenderedDOM)
if err != nil {
    hypSpan.RecordError(err)
    hypSpan.SetStatus(codes.Error, err.Error())
    hypSpan.End()
    return fail("hypothesis: %w", err)
}
// Outcome: the hypothesis JSON, subject to payload size policy.
if hypJSON, merr := json.Marshal(hyp); merr == nil {
    v, trunc := telemetry.Truncate(string(hypJSON))
    hypSpan.SetAttributes(attribute.String(telemetry.AttrPhaseOutcome, v))
    if trunc {
        hypSpan.SetAttributes(attribute.Bool(telemetry.AttrPhaseOutcome+".truncated", true))
    }
}
hypSpan.End()
```

### Phase 3 — enrichment

```go
enrichCtx, enrichSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.enrichment",
    trace.WithAttributes(
        attribute.String(telemetry.AttrPhase, "enrichment"),
        attribute.Int(telemetry.AttrPhaseIndex, 3),
    ))

enrichTrace, enrichSummary, err := agent.Run(enrichCtx, inv, llmClient, toolServer.Client, toolCB)
if err != nil {
    enrichSpan.RecordError(err)
    enrichSpan.SetStatus(codes.Error, err.Error())
    enrichSpan.End()
    return fail("enrichment: %w", err)
}
enrichSpan.End()
```

No `AttrPhaseOutcome` on the enrichment span — the tool trace is already stored in Postgres and tool-level spans carry the per-call data.

### Phase 4 — synthesis

```go
synthCtx, synthSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.synthesis",
    trace.WithAttributes(
        attribute.String(telemetry.AttrPhase, "synthesis"),
        attribute.Int(telemetry.AttrPhaseIndex, 4),
    ))

synthResult, err = synthesis.Generate(synthCtx, llmClient, inv)
if err != nil {
    synthSpan.RecordError(err)
    synthSpan.SetStatus(codes.Error, err.Error())
    synthSpan.End()
    return fail("synthesis: %w", err)
}
if synthJSON, merr := json.Marshal(synthResult); merr == nil {
    v, trunc := telemetry.Truncate(string(synthJSON))
    synthSpan.SetAttributes(attribute.String(telemetry.AttrPhaseOutcome, v))
    if trunc {
        synthSpan.SetAttributes(attribute.Bool(telemetry.AttrPhaseOutcome+".truncated", true))
    }
}
synthSpan.End()
```

---

## Span creation: internal/hypothesis/generate.go

One LLM call span is created around the single `client.Messages.New` call.

```go
tracer := otel.Tracer(telemetry.TracerName)
spanName := "chat " + string(model)
ctx, span := tracer.Start(ctx, spanName,
    trace.WithAttributes(
        attribute.String(telemetry.AttrGenAIOperationName, "chat"),
        attribute.String(telemetry.AttrGenAIProviderName, "anthropic"),
        attribute.String(telemetry.AttrGenAIRequestModel, string(model)),
        attribute.Int(telemetry.AttrGenAIRequestMaxTokens, maxTokens),
        // Screenshot — never inlined
        attribute.String(telemetry.AttrScreenshotContentType, "image/png"),
        attribute.Int64(telemetry.AttrScreenshotSizeBytes, int64(len(screenshotPNG))),
        attribute.String(telemetry.AttrScreenshotSHA256, screenshotSHA256(screenshotPNG)),
    ))
defer span.End()

resp, err := client.Messages.New(ctx, ...)
if err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
    return Hypothesis{}, fmt.Errorf("anthropic API: %w", err)
}

setLLMResponseAttrs(span, resp)
```

`screenshotSHA256` is an unexported helper that computes `sha256.Sum256(b)` and returns the hex string. `setLLMResponseAttrs` is an unexported helper in the `hypothesis` package that sets the standard response attributes from a `*anthropic.Message`:

```go
func setLLMResponseAttrs(span trace.Span, resp *anthropic.Message) {
    span.SetAttributes(
        attribute.String(telemetry.AttrGenAIResponseModel, resp.Model),
        attribute.String(telemetry.AttrGenAIResponseID, resp.ID),
        attribute.Int64(telemetry.AttrGenAIUsageInputTokens,
            resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens),
        attribute.Int64(telemetry.AttrGenAIUsageOutputTokens, resp.Usage.OutputTokens),
        attribute.Int64(telemetry.AttrGenAIUsageCacheCreationInputTokens, resp.Usage.CacheCreationInputTokens),
        attribute.Int64(telemetry.AttrGenAIUsageCacheReadInputTokens, resp.Usage.CacheReadInputTokens),
    )
    // finish_reasons — collect from StopReason
    if resp.StopReason != "" {
        span.SetAttributes(attribute.StringSlice(telemetry.AttrGenAIResponseFinishReasons, []string{string(resp.StopReason)}))
    }
}
```

**Decision: duplicate `setLLMResponseAttrs` in `hypothesis`, `synthesis`, and `agent`.** They are short and the packages cannot share a private helper without adding a dependency. The alternative — a `telemetry.SetLLMResponseAttrs(span, resp)` function in `internal/telemetry` — is ruled out: it would import the Anthropic SDK into a pure infrastructure package, coupling telemetry to a business dependency in the wrong direction. All three packages duplicate ~10 lines independently.

---

## Span creation: internal/synthesis/synthesis.go

Identical pattern to hypothesis — one LLM call span around the single `client.Messages.New` call, using the same `setLLMResponseAttrs` helper (duplicated locally).

---

## Span creation: internal/agent/run.go

Two kinds of spans per iteration: one LLM call span per Anthropic API call, one tool span per tool dispatch. Tool spans are children of the enrichment phase span (parent `ctx`), not of the LLM call span.

### LLM call span (per turn)

```go
spanName := "chat " + string(model)
llmCtx, llmSpan := otel.Tracer(telemetry.TracerName).Start(ctx, spanName,
    trace.WithAttributes(
        attribute.String(telemetry.AttrGenAIOperationName, "chat"),
        attribute.String(telemetry.AttrGenAIProviderName, "anthropic"),
        attribute.String(telemetry.AttrGenAIRequestModel, string(model)),
        attribute.Int(telemetry.AttrGenAIRequestMaxTokens, maxTokens),
    ))

resp, err := anthropicClient.Messages.New(llmCtx, anthropic.MessageNewParams{ ... })
if err != nil {
    llmSpan.RecordError(err)
    llmSpan.SetStatus(codes.Error, err.Error())
    llmSpan.End()
    return trace, summary, fmt.Errorf("anthropic API (turn %d): %w", turn+1, err)
}
setLLMResponseAttrs(llmSpan, resp)
llmSpan.End()
```

`llmCtx` is passed to `anthropicClient.Messages.New` so any HTTP-level OTel instrumentation (if added later) propagates correctly. The tool dispatch calls below use `ctx` (the enrichment phase context), not `llmCtx`.

### Tool call span (per dispatch)

Replaces the inline `dispatchTool` call site inside the content block loop:

```go
toolCtx, toolSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "execute_tool "+tu.Name,
    trace.WithAttributes(
        attribute.String(telemetry.AttrGenAIToolName, tu.Name),
    ))
if tu.ID != "" {
    toolSpan.SetAttributes(attribute.String(telemetry.AttrGenAIToolCallID, tu.ID))
}

// Input
inputV, inputTrunc := telemetry.Truncate(string(tu.Input))
toolSpan.SetAttributes(attribute.String(telemetry.AttrToolInput, inputV))
if inputTrunc {
    toolSpan.SetAttributes(attribute.Bool(telemetry.AttrToolInput+".truncated", true))
}

output, callErr := dispatchTool(toolCtx, mcpClient, tu.Name, tu.Input)

if callErr != nil {
    toolSpan.RecordError(callErr)
    toolSpan.SetStatus(codes.Error, callErr.Error())
} else {
    outputV, outputTrunc := telemetry.Truncate(string(output))
    toolSpan.SetAttributes(attribute.String(telemetry.AttrToolOutput, outputV))
    if outputTrunc {
        toolSpan.SetAttributes(attribute.Bool(telemetry.AttrToolOutput+".truncated", true))
    }
}
toolSpan.End()
```

---

## File exporter format

Use `stdouttrace.New(stdouttrace.WithWriter(f))`. By default the OTel Go stdouttrace exporter writes structured JSON per span. If the version available supports a compact (non-pretty-printed) option, use it. If not, the per-span JSON objects are still NDJSON-compatible since each is a self-contained JSON document terminated by a newline — readable line-by-line with `jq`.

**Decision: stdouttrace, not a custom exporter.** A custom exporter would give exact control over the JSON shape (e.g., OTLP proto JSON encoding), but would add ~100 lines of boilerplate for zero benefit at dev scale. stdouttrace produces human-readable output that `jq` can process; ssspy can read it directly or via a thin adapter. If ssspy later requires OTLP JSON, the file exporter is swapped at that point.

---

## Agent version

`telemetry.Version()` calls `debug.ReadBuildInfo()` and scans `info.Settings` for `{Key: "vcs.revision"}`. Returns the first 12 hex characters. Falls back to `"dev"` if:
- `debug.ReadBuildInfo()` returns `ok == false`
- No `vcs.revision` setting is present (common with `go run` and `go test`)
- The revision string is shorter than 12 characters (use the full string in that case)

---

## Error handling in instrumentation

**Rule: instrumentation errors are always silent at runtime.**

- `otel.Tracer(name)` never returns an error — it returns a valid tracer even if the provider is no-op.
- `tracer.Start(ctx, name)` never panics — returns a context and a valid span (possibly no-op).
- `span.SetAttributes(...)`, `span.RecordError(...)`, `span.SetStatus(...)`, `span.End()` — all no-ops on a no-op span.
- The only place instrumentation errors surface is during `telemetry.Init()`, where they are logged to stderr (not to the investigation error path).

No instrumentation code should use `log.Fatal`, `os.Exit`, or `panic`. The investigation process exit code is never influenced by instrumentation state.

---

## Payload size — implementation detail

Multimodal content (the Phase 2 screenshot) is handled at the attribute-setting callsite in `hypothesis/generate.go`. The raw PNG bytes are available there; computing the SHA-256 and byte count requires no additional I/O. The base64-encoded form is what gets sent to the API; the size attribute records `len(screenshotPNG)` (the raw bytes), not the base64 size.

---

## Open decisions resolved here

**Global tracer vs. injected tracer.** Global tracer (`otel.Tracer(...)`). The alternative is threading a `trace.Tracer` through every function signature — this would touch six packages and add a parameter to `pipeline.Run`, `agent.Run`, `hypothesis.Generate`, and `synthesis.Generate`. The global is the idiomatic OTel Go pattern and avoids signature churn. The provider is set once in `main.go` before any pipeline code runs.

**stdouttrace vs. custom file exporter.** stdouttrace (see file exporter section above).

**Duplicate `setLLMResponseAttrs` vs. shared function in `internal/telemetry`.** Duplicated in `hypothesis`, `synthesis`, and `agent`. Rationale: keeping `internal/telemetry` free of Anthropic SDK imports keeps it pure infrastructure. All three packages duplicate ~10 lines independently.

**Tool spans as siblings of LLM spans (not children).** Requirements say tools are children of the enrichment phase span. This means tool spans are created from `ctx` (phase context), not from `llmCtx`. The LLM call span is a sibling of the tool spans within the enrichment phase.

**`setLLMResponseAttrs` uses `StopReason` for finish reasons.** The Anthropic SDK returns a single `StopReason` string, not a slice. The semconv attribute is a string slice; we wrap the single value in a one-element slice. If the SDK is updated to return multiple stop reasons, update accordingly.

**Phase span name format: `ssspy.phase.{phase}`.** Alternative considered: human-readable names like `"Phase 1: fetch"`. The `ssspy.*` format is more machine-filterable and consistent with the `ssspy.*` attribute namespace. Trace viewers can always alias span names in their UI.

**No outcome attribute on fetch and enrichment phase spans.** Fetch artifacts are large and already in Postgres; adding them to a span attribute adds cost with no analytical value. Enrichment trace is per-tool-call, already captured on tool spans. Only phases that produce a compact structured output (hypothesis, synthesis) carry `ssspy.investigation.outcome`.

**Stub mode (`--skip-llm`) span behaviour.** When `skipLLM=true`, `pipeline.go` still creates and exports the root span and all four phase spans as normal. No LLM call spans or tool call spans are emitted because those calls simply do not happen — there is nothing to observe. No fake/stub child spans are injected. The requirement that "instrumentation still runs" is satisfied by the root and phase spans; the absent LLM and tool spans are an accurate reflection of what the pipeline actually executed.

**Content is not redacted.** Full prompts, tool outputs, and page content are emitted as-is. This is a personal/local deployment; no PII policy applies. This assumption is documented in `CONTRACT.md`.

---

## Files changed

| File | Change |
|---|---|
| `internal/telemetry/tracer.go` | New — tracer init/shutdown, Version() |
| `internal/telemetry/attrs.go` | New — all attribute constants |
| `internal/telemetry/payload.go` | New — Truncate() helper |
| `cmd/gophish/main.go` | Add telemetry.Init, root span, shutdown |
| `cmd/server/main.go` | Add telemetry.Init, shutdown (no spans) |
| `internal/pipeline/pipeline.go` | Add phase spans; pass phase-scoped ctx to sub-calls |
| `internal/hypothesis/generate.go` | Add LLM call span, screenshot attrs |
| `internal/synthesis/synthesis.go` | Add LLM call span |
| `internal/agent/run.go` | Add LLM call spans per turn, tool spans per dispatch |
| `go.mod` / `go.sum` | Add four OTel dependencies |
| `CONTRACT.md` | New — emitted span schema reference |

---

## Out of scope

Anything not listed above. No metrics, no logs, no sampling configuration, no distributed context propagation, no HTTP middleware spans for the web server, no ssspy-side work.
