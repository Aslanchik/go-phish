# enrichment-tools: Design

## Overview

This document specifies the architecture for Phase 3: the agent loop, the MCP tool server, and the five enrichment tools. Implementation follows this document; prompt text does not appear here.

---

## Package structure

The two stubs that already exist grow into real implementations:

```
internal/agent/       — agent loop: builds messages, dispatches tool calls, owns the iteration cap
internal/tools/       — MCP server + individual tool implementations
```

`cmd/gophish/main.go` gains one new pipeline step between hypothesis and synthesis: `agent.Run(...)`.

No other packages change for this feature.

---

## MCP tool server

### Technology choice: mark3labs/mcp-go over SSE transport

**Decision:** use `github.com/mark3labs/mcp-go` with an SSE (HTTP) transport, listening on `localhost:0` (OS-assigned port).

Tools are registered with the MCP server at startup using `mcp-go`'s registration API. The server starts in a goroutine. An MCP client connects to it over localhost HTTP. The agent loop uses the client to:
1. List available tools → convert to `anthropic.ToolUnionParam` definitions for the Anthropic API
2. Dispatch tool calls from Claude's `tool_use` blocks → get results → convert to `anthropic.ToolResultBlockParam`

This is the "bridge" layer between the Anthropic tool_use protocol and MCP, and it lives in `internal/agent/`.

Alternatives considered:

| Option | Pro | Con |
|---|---|---|
| SSE transport (chosen) | Real MCP protocol; debuggable with standard HTTP tools; easy to extract to a standalone server later | Loopback socket overhead (negligible) |
| stdio via `io.Pipe` | No network, faster | Less representative of how MCP is used in practice; harder to debug |
| Direct function dispatch (no MCP) | Simplest code | Doesn't practice MCP at all; defeats the learning objective |

The SSE approach is chosen because it exercises the real protocol while keeping everything in one process. The server never binds to anything reachable outside localhost.

### Tool server constructor

```go
// internal/tools/server.go
type Server struct { ... }

func New(anthropicClient *anthropic.Client) (*Server, error)
func (s *Server) URL() string                // http://localhost:<port>
func (s *Server) Stop(ctx context.Context) error
```

The Anthropic client is injected so `analyze_js` can make its own LLM calls without a global.

### Dependency to add

- `github.com/mark3labs/mcp-go` — MCP Go SDK (most widely used Go MCP implementation)

---

## Agent loop

### Architecture

```go
// internal/agent/run.go

func Run(
    ctx         context.Context,
    conn        *sql.DB,
    inv         db.Investigation,
    anthropic   *anthropic.Client,
    mcpURL      string,        // tool server address
) ([]ToolCall, error)
```

The loop:

1. Connect MCP client to `mcpURL`, list tools, convert to Anthropic tool definitions
2. Build the initial message: Phase 2 hypothesis + Phase 1 artifact summary (final URL, form actions, count of JS files) as a text block
3. Call Claude with `tool_choice: auto` — the model decides what to call and when
4. For each `tool_use` block in the response, dispatch to the MCP client and collect results
5. Append the assistant turn and all tool results as a new user message
6. Repeat from step 3 until: (a) the response contains no `tool_use` blocks, or (b) the iteration cap is reached
7. Return the accumulated call trace

### Iteration cap

Default: **10 turns**. Configurable via `ENRICHMENT_MAX_TURNS` environment variable. If the cap is reached, the loop ends and the call trace up to that point is persisted — it is not an error. The final response text (if any) is stored as the enrichment summary.

Rationale for 10: five tools, a model that calls each once ends in 5 turns. 10 leaves room for follow-up calls (e.g. cert_transparency on a subdomain after whois reveals a pattern) without runaway loops.

### ToolCall record

```go
type ToolCall struct {
    Tool     string          `json:"tool"`
    Input    json.RawMessage `json:"input"`
    Output   json.RawMessage `json:"output"`
    CalledAt time.Time       `json:"called_at"`
}
```

This is what gets serialised into `enrichment_trace` in Postgres.

### Handling tool errors

If a tool call fails (network error, bad arguments, external API down), the error is returned to the model as a structured tool result:

```json
{ "error": "whois query timed out after 10s", "tool": "whois_lookup" }
```

The loop does not abort. The model receives the error and decides what to do next — it may retry, call a different tool, or proceed to synthesis.

---

## Tool implementations

All tools live in `internal/tools/`. Each is independently testable: inputs and outputs are plain Go structs; no MCP or HTTP is involved in the unit tests.

### whois_lookup

```
Input:  { "domain": "string" }
Output: { "registrar": "string", "registered_at": "string (RFC3339)", "expires_at": "string (RFC3339)",
          "registrant_org": "string", "raw": "string" }
```

Implementation: `github.com/likexian/whois` for the raw query, `github.com/likexian/whois-parser` for structured extraction. `registered_at` is surfaced as a top-level field — it is the primary signal for freshly-registered phishing domains.

Timeout: 10 seconds. Returned as a structured error result if exceeded.

Dependencies to add:
- `github.com/likexian/whois`
- `github.com/likexian/whois-parser`

### cert_transparency

```
Input:  { "domain": "string" }
Output: { "certificates": [ { "common_name": "string", "san_entries": ["string"],
          "issuer": "string", "not_before": "string", "not_after": "string" } ] }
```

Implementation: HTTP GET to `https://crt.sh/?q=<domain>&output=json`. Uses standard library `net/http` — no new dependency. Results are capped at 50 entries to avoid overwhelming the context window. If crt.sh is unreachable or returns no results, a structured result with an empty list (or error field) is returned.

### urlscan_lookup

```
Input:  { "url": "string" }
Output: { "scans": [ { "scan_date": "string", "verdict": "string",
          "tags": ["string"], "page_url": "string" } ] }
```

Implementation: HTTP GET to `https://urlscan.io/api/v1/search/?q=page.url:<url>`. API key read from `URLSCAN_API_KEY` environment variable. If not set, returns a structured error (`"error": "URLSCAN_API_KEY not set"`). Results capped at 10 entries. Uses standard library `net/http`.

### urlhaus_check

```
Input:  { "url_or_domain": "string" }
Output: { "found": bool, "threat_type": "string", "tags": ["string"],
          "date_added": "string", "urls_on_host": ["string"] }
```

Implementation: HTTP POST to `https://urlhaus-api.abuse.ch/v1/url/` (for full URLs) or `https://urlhaus-api.abuse.ch/v1/host/` (for domains). No API key required. If the entry is not found, `found: false` is returned — not an error. `urls_on_host` is capped at 20 entries.

### analyze_js

```
Input:  { "js_content": "string" }
Output: { "kit_name": "string", "exfil_urls": ["string"], "obfuscation_detected": bool,
          "notable_strings": ["string"], "summary": "string" }
```

Implementation: Anthropic API call (same client as the main pipeline) with a system prompt instructing the model to analyse the JS for phishing kit indicators. The JS is passed as a text block.

Token budget: if `js_content` exceeds **50,000 characters** (~12,500 tokens), it is truncated to that length and the output includes `"truncated": true`.

This tool sends phishing kit source code to the Anthropic API. This is documented in the CLI `--help` text and in the safety section below — it is not hidden behaviour.

---

## Postgres changes

### New migration: `0003_add_enrichment_trace.sql`

```sql
-- +goose Up
ALTER TABLE investigations
    ADD COLUMN enrichment_trace JSONB,
    ADD COLUMN enrichment_summary TEXT;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN enrichment_trace,
    DROP COLUMN enrichment_summary;
```

`enrichment_trace` stores the ordered array of `ToolCall` records (see above). `enrichment_summary` stores the final text response from the model when it signals completion.

### New status value: `enriching`

The `status` column gains a new value: `enriching`, inserted between `hypothesizing` and `complete`:

```
pending → fetching → hypothesizing → enriching → complete
                                              ↘ failed
```

No schema migration needed — `status` is already a free-form TEXT column. A new constant is added to `internal/db/status.go`.

### New CRUD functions in `internal/db/`

```go
func UpdateEnrichment(ctx context.Context, conn *sql.DB, id string, trace []agent.ToolCall, summary string) error
```

---

## Failure modes

| Failure | Behaviour |
|---|---|
| MCP server fails to start | Pipeline fails before the agent loop begins |
| Tool call returns structured error | Error passed to model as tool result; loop continues |
| External API unreachable (whois, crt.sh, etc.) | Structured error returned to model; loop continues |
| `analyze_js` LLM call fails | Structured error returned to model; loop continues |
| Iteration cap reached | Loop ends; partial trace persisted; not treated as an error |
| DB unavailable when persisting trace | Pipeline fails with descriptive error; trace is lost — **known limitation** |
| Model calls unknown tool name | MCP client returns structured error; loop continues |

---

## Open decisions resolved here

**MCP transport:** SSE over localhost (see Technology choice section).

**Iteration cap:** 10 turns, configurable via `ENRICHMENT_MAX_TURNS`.

**Bad tool arguments:** returned to model as structured error, loop continues. We let the model learn from its mistakes rather than aborting — this is a deliberate choice to surface failure modes.

**Token budget for `analyze_js`:** 50,000 characters.

**Virustotal, compare_to_brand_login, analyze_form:** out of scope for this iteration (see requirements.md).

---

## Safety

- **S-5** (read-only tools): all tool implementations make GET/POST queries to read-only public APIs. No write operations.
- **S-6** (no credentials in tool output): tool output fields are validated before being returned. Raw API responses are not passed directly — they are unmarshalled into typed structs first.
- **S-7** (`analyze_js` sends data externally): the CLI `--help` text includes the note: "The analyze_js enrichment tool sends JavaScript file content to the Anthropic API for analysis." Operators can disable the tool by setting `DISABLE_ANALYZE_JS=1` — the tool server will register it but return a structured error result if called.

---

## Out of scope

Anything not listed above. No eval harness changes, no Phase 4 synthesis, no UI.
