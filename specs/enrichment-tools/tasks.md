# enrichment-tools: Tasks

Tasks are in three phases. Phases A and C are sequential. Phase B tasks are independent of each other and intended to run in parallel (one sub-agent per tool, each in an isolated git worktree).

**Do not start Phase B until all Phase A tasks are complete and merged to main.**
**Do not start Phase C until all Phase B PRs are merged.**

Status: `[ ]` todo · `[x]` done · `[~]` in progress

---

## Phase A — Foundation (sequential)

### [ ] ET-F1: Add dependencies

**Satisfies:** ET-2 (MCP server), ET-3 (whois_lookup)

In the main module (`go.mod` at repo root):

```bash
go get github.com/mark3labs/mcp-go
go get github.com/likexian/whois
go get github.com/likexian/whois-parser
```

**Verified when:** `go mod tidy` succeeds and all three packages appear in `go.sum`; `go build ./...` still passes.

---

### [ ] ET-F2: Migration 0003 — enrichment columns

**Satisfies:** ET-8

Write `internal/db/migrations/0003_add_enrichment_trace.sql`:

```sql
-- +goose Up
ALTER TABLE investigations
    ADD COLUMN enrichment_trace   JSONB,
    ADD COLUMN enrichment_summary TEXT;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN enrichment_trace,
    DROP COLUMN enrichment_summary;
```

**Verified when:** running migrations against a local Postgres instance adds both columns to `investigations`; rolling back removes them cleanly.

---

### [ ] ET-F3: db — enriching status + UpdateEnrichment

**Satisfies:** ET-8

- Add `StatusEnriching Status = "enriching"` to `internal/db/status.go`
- Add to `internal/db/investigations.go`:

```go
func UpdateEnrichment(ctx context.Context, conn *sql.DB, id string, trace json.RawMessage, summary string) error
```

Stores `enrichment_trace` and `enrichment_summary` on the investigations row.

- Add `EnrichmentTrace json.RawMessage` and `EnrichmentSummary sql.NullString` fields to the `Investigation` struct.
- Update `GetInvestigation` to scan the two new columns.

**Verified when:** `UpdateEnrichment` sets both columns on an existing row; `GetInvestigation` returns them correctly.

---

### [ ] ET-F4: MCP server scaffold

**Satisfies:** ET-2

Create `internal/tools/server.go`:

```go
type Server struct { ... }

// New creates and starts the MCP server on localhost:0.
// The Anthropic client is injected for use by the analyze_js tool.
func New(ctx context.Context, anthropicClient *anthropic.Client) (*Server, error)

// URL returns the base URL of the running server (e.g. http://localhost:54321).
func (s *Server) URL() string

// Stop shuts down the server gracefully.
func (s *Server) Stop(ctx context.Context) error
```

- Use `mark3labs/mcp-go` with SSE transport
- The server starts in a goroutine and is ready before `New` returns
- Tool handlers are registered in `New` via a `registerTools(s *mcp.Server, client *anthropic.Client)` helper (initially empty — tools are added in Phase B)
- Export the `registerTools` function signature in a comment so Phase B agents know the pattern

**Verified when:** `New` starts a server; `URL()` returns a reachable address; `Stop` shuts it down cleanly; `go test ./internal/tools/` passes.

---

### [ ] ET-F5: Agent loop

**Satisfies:** ET-1

Create `internal/agent/run.go`:

```go
type ToolCall struct {
    Tool     string          `json:"tool"`
    Input    json.RawMessage `json:"input"`
    Output   json.RawMessage `json:"output"`
    CalledAt time.Time       `json:"called_at"`
}

func Run(
    ctx           context.Context,
    inv           db.Investigation,
    anthropic     *anthropic.Client,
    mcpServerURL  string,
) (trace []ToolCall, summary string, err error)
```

Implement the loop:
1. Connect an MCP client to `mcpServerURL`, list tools, convert each to `anthropic.ToolUnionParam`
2. Build the initial user message: hypothesis JSON + Phase 1 artifact summary (final URL, form actions, JS file count) as a single text block
3. Call Claude with `tool_choice: auto` and `model: claude-sonnet-4-6`
4. For each `tool_use` block: dispatch to the MCP client, collect the result as a `ToolCall`, append to trace
5. Build a new user message from all `tool_result` blocks; repeat from step 3
6. Stop when the response contains no `tool_use` blocks (capture the final text as `summary`) or when the iteration cap is reached
7. Iteration cap: read `ENRICHMENT_MAX_TURNS` env var; default 10

Error handling: tool errors are returned to the model as structured tool results (not Go errors), per design.md. Only a failed MCP connection or a failed Anthropic call should propagate as a Go error.

**Verified when:** with a mock MCP server that returns canned responses, the loop terminates on a text-only response; it also terminates at the cap and returns a partial trace; tool errors are passed back to the model rather than aborting.

---

### [ ] ET-F6: Wire Phase 3 into main pipeline

**Satisfies:** ET-1, ET-8

In `cmd/gophish/main.go`, after `UpdateHypothesis`:

1. Start the tool server: `tools.New(ctx, anthropicClient)`
2. Update status to `enriching`
3. Call `agent.Run(ctx, inv, anthropicClient, toolServer.URL())`
4. Marshal trace to `json.RawMessage`, call `db.UpdateEnrichment`
5. Update status to `complete` (synthesis is not built yet — skip straight to complete for now)
6. Stop the tool server

On any error: update status to `failed`, store error message, return.

**Verified when:** `gophish <url>` runs end-to-end with an empty tool server (no tools registered yet); the pipeline reaches `complete`; `enrichment_trace` is `[]` in Postgres (empty array, not null).

---

## Phase B — Tools (parallel)

**These five tasks are independent. Run each in its own git worktree on its own branch.**
**Each agent should branch from the Phase A merge commit on main.**
**Each agent opens a PR when done. Do not merge until all five are open for review.**

The MCP tool registration pattern (established in ET-F4) is:

```go
// In internal/tools/server.go, inside registerTools:
s.AddTool(
    mcp.NewTool("<tool_name>",
        mcp.WithDescription("..."),
        mcp.WithString("<param>", mcp.Required(), mcp.Description("...")),
    ),
    handler,
)
```

Handler signature: `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`

Return results as JSON text: `mcp.NewToolResultText(jsonStr)`. Return errors as JSON: `mcp.NewToolResultText(`{"error":"..."}`)` — do **not** return a Go error (that signals an MCP protocol failure, not a tool failure).

---

### [ ] ET-T1: whois_lookup tool

**Satisfies:** ET-3

Create `internal/tools/whois.go`.

Input schema:
```json
{ "domain": "string" }
```

Output schema:
```json
{
  "registrar":       "string",
  "registered_at":   "string (RFC3339, empty if unknown)",
  "expires_at":      "string (RFC3339, empty if unknown)",
  "registrant_org":  "string (empty if redacted)",
  "raw":             "string"
}
```

Implementation:
- Use `github.com/likexian/whois` for the raw query (10-second timeout via context)
- Use `github.com/likexian/whois-parser` for structured extraction
- If the query times out or the domain has no record, return `{"error": "<reason>"}` as the tool result

Register in `internal/tools/server.go` inside `registerTools`.

Write `internal/tools/whois_test.go` with at least:
- A unit test for the output struct serialisation
- A test that a timeout produces a structured error result (not a panic)

**Verified when:** `go test ./internal/tools/ -run TestWhois` passes; `go vet ./internal/tools/` passes.

---

### [ ] ET-T2: cert_transparency tool

**Satisfies:** ET-4

Create `internal/tools/crtsh.go`.

Input schema:
```json
{ "domain": "string" }
```

Output schema:
```json
{
  "certificates": [
    {
      "common_name": "string",
      "san_entries": ["string"],
      "issuer":      "string",
      "not_before":  "string (RFC3339)",
      "not_after":   "string (RFC3339)"
    }
  ]
}
```

Implementation:
- HTTP GET to `https://crt.sh/?q=<domain>&output=json` using `net/http` (15-second timeout)
- Parse the JSON array response; map fields to the output schema
- Cap results at 50 entries
- If crt.sh is unreachable or returns a non-200 status, return `{"error": "<reason>", "certificates": []}` as the tool result

Register in `internal/tools/server.go` inside `registerTools`.

Write `internal/tools/crtsh_test.go` with at least:
- A unit test using an `httptest.Server` that returns a canned crt.sh JSON payload, verifying field mapping and the 50-entry cap
- A test that an HTTP error returns a structured error result

**Verified when:** `go test ./internal/tools/ -run TestCertTransparency` passes; `go vet ./internal/tools/` passes.

---

### [ ] ET-T3: urlscan_lookup tool

**Satisfies:** ET-5

Create `internal/tools/urlscan.go`.

Input schema:
```json
{ "url": "string" }
```

Output schema:
```json
{
  "scans": [
    {
      "scan_date": "string (RFC3339)",
      "verdict":   "string (malicious|suspicious|benign|unknown)",
      "tags":      ["string"],
      "page_url":  "string"
    }
  ]
}
```

Implementation:
- Read `URLSCAN_API_KEY` from environment; if not set, return `{"error": "URLSCAN_API_KEY not set"}` as the tool result
- HTTP GET to `https://urlscan.io/api/v1/search/?q=page.url:<url>` with `Authorization: API-Key <key>` header (15-second timeout)
- Parse response; map fields; cap at 10 results
- If the API returns no results, return `{"scans": []}` — not an error

Register in `internal/tools/server.go` inside `registerTools`.

Write `internal/tools/urlscan_test.go` with at least:
- A unit test using an `httptest.Server` with a canned response, verifying field mapping and 10-entry cap
- A test that a missing API key returns a structured error result

**Verified when:** `go test ./internal/tools/ -run TestURLScan` passes; `go vet ./internal/tools/` passes.

---

### [ ] ET-T4: urlhaus_check tool

**Satisfies:** ET-6

Create `internal/tools/urlhaus.go`.

Input schema:
```json
{ "url_or_domain": "string" }
```

Output schema:
```json
{
  "found":         false,
  "threat_type":   "string",
  "tags":          ["string"],
  "date_added":    "string (RFC3339)",
  "urls_on_host":  ["string"]
}
```

Implementation:
- If input looks like a full URL (starts with `http://` or `https://`): POST to `https://urlhaus-api.abuse.ch/v1/url/` with form body `url=<value>`
- Otherwise (bare domain): POST to `https://urlhaus-api.abuse.ch/v1/host/` with form body `host=<value>`
- Parse the JSON response; if `query_status` is `"no_results"`, return with `found: false` and empty fields — not an error
- Cap `urls_on_host` at 20 entries
- 15-second timeout

Register in `internal/tools/server.go` inside `registerTools`.

Write `internal/tools/urlhaus_test.go` with at least:
- A unit test using an `httptest.Server` with a canned "found" response, verifying `found: true` and field mapping
- A test that a `no_results` response returns `found: false` without error

**Verified when:** `go test ./internal/tools/ -run TestURLhaus` passes; `go vet ./internal/tools/` passes.

---

### [ ] ET-T5: analyze_js tool

**Satisfies:** ET-7

Create `internal/tools/analyzejs.go`.

Input schema:
```json
{ "js_content": "string" }
```

Output schema:
```json
{
  "kit_name":             "string (empty if not identifiable)",
  "exfil_urls":           ["string"],
  "obfuscation_detected": false,
  "notable_strings":      ["string"],
  "summary":              "string",
  "truncated":            false
}
```

Implementation:
- If `js_content` exceeds 50,000 characters, truncate to 50,000 and set `truncated: true` in the output
- Call Claude Sonnet (`claude-sonnet-4-6`) via the injected Anthropic client
- Force structured output via a single `record_js_analysis` tool (same pattern as `record_hypothesis` in Phase 2) with the output schema above
- If the Anthropic call fails, return `{"error": "<reason>"}` as the tool result
- If `DISABLE_ANALYZE_JS=1` is set in the environment, return `{"error": "analyze_js is disabled (DISABLE_ANALYZE_JS=1)"}` without making an API call

Register in `internal/tools/server.go` inside `registerTools`. The `*anthropic.Client` is available via the `Server` struct.

Write `internal/tools/analyzejs_test.go` with at least:
- A test that input over 50,000 characters is truncated and `truncated: true` is set
- A test that `DISABLE_ANALYZE_JS=1` returns the expected structured error without calling the API

**Verified when:** `go test ./internal/tools/ -run TestAnalyzeJS` passes; `go vet ./internal/tools/` passes.

---

## Phase C — Integration (sequential)

### [ ] ET-I1: Register all tools and smoke test

**Satisfies:** ET-1, ET-2, ET-3, ET-4, ET-5, ET-6, ET-7

After all Phase B PRs are merged:

- Verify `registerTools` in `internal/tools/server.go` calls all five tool registrations (one per Phase B task)
- Run `go build ./...` and `go test ./...` — all must pass
- Run `gophish <real-phishing-url>` with all environment variables set; verify:
  - Pipeline reaches `complete`
  - `enrichment_trace` in Postgres contains at least one tool call
  - `enrichment_summary` is non-empty
  - Report prints to stdout without panic

**Verified when:** end-to-end run completes; enrichment columns are populated; all tests pass.

---

### [ ] ET-I2: Update docs

**Satisfies:** CLAUDE.md working agreement

- Update `docs/architecture.md`: change Phase 3 status from "planned" to "built"; update the pipeline flowchart if the tool server or agent loop changed the flow
- Update `docs/data-model.md`: add `enrichment_trace` and `enrichment_summary` columns to the `investigations` table; add `enriching` to the status state-machine diagram
- Note any agent behaviour observations in `agent-notes.md` (tool call ordering, unexpected tool use, errors the model recovered from)

**Verified when:** diagrams reflect the current state of the code.
