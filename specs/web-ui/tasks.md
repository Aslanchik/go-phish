# web-ui: Tasks

Tasks are ordered. Each must be complete and verifiable before the next begins. If a task surfaces a spec problem, stop — update the relevant spec, re-review, then continue.

Status: `[ ]` todo · `[x]` done · `[~]` in progress

---

## WU-T1: Add ListInvestigations DB query

**Satisfies:** WU-5, WU-6

Add `ListInvestigations(ctx, conn)` to `internal/db/investigations.go`. The query selects `id`, `url`, `created_at`, `status`, and `synthesis` from `investigations` ordered by `created_at DESC`. Returns `[]Investigation`.

**Verified when:** `go build ./internal/db/...` succeeds and a unit test (or manual query check) confirms rows are returned in descending creation order.

---

## WU-T2: Extract pipeline run function to internal/pipeline

**Satisfies:** S-WU-1

Extract the core investigation logic from `cmd/gophish/main.go` into a new package `internal/pipeline` as `func Run(ctx context.Context, invID string, conn *sql.DB, llmClient *anthropic.Client, mcpClient *mcpclient.Client) error`. The CLI `cmd/gophish/main.go` calls this function unchanged. The extracted function must accept a progress callback parameter `func(event Event)` (where `Event` is defined in this package) so the server can hook into phase transitions and tool calls without modifying the pipeline logic.

The `Event` type carries: `InvestigationID string`, `Type string` (values: `phase_transition`, `tool_call`, `tool_result`, `log`, `complete`, `failed`), `Timestamp time.Time`, and `Data map[string]any`.

If the callback is nil, the function proceeds with no event emission.

**Verified when:** `go build ./...` succeeds, `cmd/gophish/main.go` compiles unchanged, and a manually traced run with `--skip-llm` produces the same DB state as before the refactor.

---

## WU-T3: Wire tool_call and tool_result events from agent loop

**Satisfies:** WU-2, S-WU-1

Extend `internal/agent.Run` to accept an optional event callback `func(toolName string, input json.RawMessage, output json.RawMessage)` — called once before dispatching (tool_call) and once after receiving the result (tool_result). The callback signature keeps `internal/agent` free of knowledge of the SSE envelope type; the pipeline package wraps it into an `Event` before forwarding to the progress callback.

Emit a `tool_call` event before `dispatchTool` is called and a `tool_result` event after it returns. The `tool_result` data carries a `summary` field: the first 200 characters of the output string, not the full raw output.

**Verified when:** `go build ./internal/agent/...` succeeds and a test that supplies a callback records one tool_call event and one tool_result event per tool dispatch.

---

## WU-T4: Create internal/api package skeleton

**Satisfies:** WU-7, WU-8

Create `internal/api/` with:
- `server.go` — `type Server struct` holding `*sql.DB`, `*anthropic.Client`, a reference to the SSE broker (added in WU-T5), and an `http.ServeMux`. Exports `New(db *sql.DB, ...) *Server` and `func (s *Server) Handler() http.Handler`.
- `middleware.go` — a single `safeError` helper that logs the full error server-side and writes `{"error": "internal server error"}` with status 500 to the client. Used by all handlers to avoid leaking stack traces or DSN strings.
- `routes.go` — registers all five `/api/v1/` routes on the mux plus the SPA fallback (placeholder handlers returning 501 until later tasks fill them in).

No router dependency is added. `net/http` `ServeMux` with Go 1.22 method+pattern syntax is used throughout.

**Verified when:** `go build ./internal/api/...` succeeds.

---

## WU-T5: Implement SSE broker

**Satisfies:** WU-2

Create `internal/api/broker.go`. The broker:
- Holds a map of `investigationID -> []chan Event` (subscriber channels).
- Exports `func (b *Broker) Subscribe(invID string) (<-chan Event, func())` — returns a read channel and an unsubscribe function (closing the channel and removing it from the map). Safe for concurrent use.
- Exports `func (b *Broker) Publish(e Event)` — fans the event out to all subscribers for that investigation ID. Non-blocking: if a subscriber channel is full, the event is dropped for that subscriber only (channel buffer size: 32).
- Assigns a monotonic sequence number to each event per investigation. On reconnect, the server reads `Last-Event-ID` from the request and only sends events with a higher sequence number from an in-memory ring buffer (max 32 events per investigation, retained for the lifetime of the investigation goroutine).

**Verified when:** A unit test subscribes two clients to the same investigation, publishes three events, and asserts both clients receive all three events in order. A second test verifies a slow subscriber (full channel) does not block the publisher.

---

## WU-T6: Implement POST /api/v1/investigations handler

**Satisfies:** WU-1, WU-7, WU-8, S-WU-1, S-WU-2

In `internal/api/handlers_investigations.go`, implement the POST handler:
- Parse `{"url": "..."}` from the request body.
- Validate: non-empty, must parse with `url.Parse`, must have `http` or `https` scheme and non-empty host. Return 400 with `{"error": "..."}` on failure; no investigation is created.
- Call `db.CreateInvestigation` and `db.UpdateStatus(..., db.StatusPending, "")`.
- Launch `pipeline.Run` in a goroutine, passing the SSE broker's `Publish` as the progress callback.
- Return 202 with `{"id": "...", "status": "pending"}`.
- No endpoint or code path exists that triggers form submission or any action beyond page load — the goroutine calls the same pipeline as the CLI.

**Verified when:** `curl -X POST http://localhost:8080/api/v1/investigations -d '{"url":"https://example.com"}'` returns HTTP 202 with a JSON body containing `id` and `status`. A request with `{"url":"not-a-url"}` returns HTTP 400.

---

## WU-T7: Implement GET /api/v1/investigations handler

**Satisfies:** WU-5, WU-7, WU-8

Implement the GET list handler. Calls `db.ListInvestigations`, marshals the result to JSON. Each item in the response array includes: `id`, `url`, `status`, `created_at`, and `verdict` (extracted from `synthesis.verdict.value` if synthesis is present, otherwise omitted or `null`). Returns 200 with the array (empty array, not null, when no investigations exist). Uses `safeError` for any DB failure.

**Verified when:** `curl http://localhost:8080/api/v1/investigations` returns a JSON array. After submitting one investigation via WU-T6, the array contains that investigation.

---

## WU-T8: Implement GET /api/v1/investigations/:id handler

**Satisfies:** WU-3, WU-7, WU-8

Implement the GET single investigation handler. Calls `db.GetInvestigation`. Returns 404 if the ID is not found. Response includes all fields needed by the report view: `id`, `url`, `created_at`, `status`, `hypothesis` (JSON object), `synthesis` (JSON object or null), `enrichment_summary`. Does not include `screenshot` bytes (served by the dedicated endpoint in WU-T9). Uses `safeError` for DB errors; never includes DSN or stack traces in the response.

**Verified when:** `curl http://localhost:8080/api/v1/investigations/<id>` returns a JSON object for a known ID and a `{"error": "not found"}` with status 404 for an unknown ID.

---

## WU-T9: Implement GET /api/v1/investigations/:id/screenshot handler

**Satisfies:** WU-4, WU-7, S-WU-3

Implement the screenshot handler. Reads `investigations.screenshot` (the `[]byte` column) via `db.GetInvestigation`. If the column is non-nil and non-empty, writes it with `Content-Type: image/png` and status 200. If nil or empty, returns 404. The handler never fetches from the original URL — bytes come exclusively from the DB column.

**Verified when:** `curl -I http://localhost:8080/api/v1/investigations/<id>/screenshot` returns `Content-Type: image/png` and 200 for an investigation with a screenshot, and 404 for one without.

---

## WU-T10: Implement GET /api/v1/investigations/:id/events SSE handler

**Satisfies:** WU-2, WU-7

Implement the SSE handler. Sets headers `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`. Reads `Last-Event-ID` from the request header; calls `broker.Subscribe` and replays any buffered events with a higher sequence number before blocking on new ones. Formats each `Event` as SSE: `id: <seq>\ndata: <json>\n\n`. Closes the stream on context cancellation (client disconnect) or when a `complete` or `failed` event is received.

**Verified when:** Opening `http://localhost:8080/api/v1/investigations/<id>/events` in a browser `EventSource` or via `curl --no-buffer` while a pipeline is running shows a stream of `data:` lines that terminates with a `complete` or `failed` event.

---

## WU-T11: Create cmd/server/main.go

**Satisfies:** WU-1, WU-7

Create `cmd/server/main.go`. Parses `--addr` flag (default `:8080`). Calls `db.Open()`, initialises the Anthropic client (skips if `ANTHROPIC_API_KEY` is unset, logging a warning). Constructs `api.New(...)` and calls `http.ListenAndServe(addr, server.Handler())`. Logs the listening address.

This is the only place that wires `internal/api` to `internal/db` and `internal/pipeline`. The existing `cmd/gophish/main.go` is unchanged.

**Verified when:** `go build ./cmd/server/` succeeds (binary removed after build). Running the server binary starts an HTTP server that responds to `curl http://localhost:8080/api/v1/investigations`.

---

## WU-T12: Scaffold web/ directory with Vite + React + Tailwind + shadcn/ui

**Satisfies:** WU-1, WU-3, WU-5

**Decision point (flagged, not re-opened):** The design mandates React + Vite + Tailwind + shadcn/ui. No alternative is evaluated here. Before executing this task, confirm that adding `web/` as a Node.js subproject (with its own `package.json` and `node_modules`) is acceptable — it is not a Go dependency and does not appear in `go.mod`.

Run `npm create vite@latest web -- --template react-ts` from the repo root. Install Tailwind CSS and shadcn/ui following their standard setup for Vite projects. Configure `web/vite.config.ts` to:
- Output build to `web/dist/`
- Proxy `/api/` to `http://localhost:8080` during dev

Add `web/dist/` to `.gitignore` (assets are generated at build time, not committed).

**Verified when:** `cd web && npm run dev` starts a Vite dev server without errors, and `npm run build` produces a `web/dist/` directory containing `index.html`.

---

## WU-T13: Implement SubmitForm and InvestigationList components on /

**Satisfies:** WU-1, WU-5, WU-6

In `web/src/`, create:
- `components/SubmitForm.tsx` — URL text input with client-side validation (non-empty; must parse as a URL with scheme and host using `new URL()`; validation fires on submit, not on change). On submit, `POST /api/v1/investigations`. Shows a loading state while the request is in flight. On success, navigates to `/investigations/:id`.
- `components/InvestigationList.tsx` — fetches `GET /api/v1/investigations` on mount. Renders a table with columns: URL, status/verdict, timestamp. Rows with `status === "failed"` receive a distinct visual treatment (e.g. a red badge). Each row links to `/investigations/:id`.
- `pages/HomePage.tsx` — composes `SubmitForm` above `InvestigationList`.
- Wire `/` to `HomePage` in the router.

React Router (already included in the Vite React template as a common addition, or added here — flag as a dependency decision if not already present) handles client-side routing.

**Verified when:** Navigating to `http://localhost:5173/` (Vite dev server) shows the submission form and the investigation list. Submitting a valid URL redirects to `/investigations/<id>`. Submitting an empty string or `"notaurl"` shows a validation error without sending a network request (confirm in browser devtools network tab).

---

## WU-T14: Implement ReportView component on /investigations/:id

**Satisfies:** WU-3, WU-4, WU-6

Create `web/src/components/ReportView.tsx`. On mount, fetches `GET /api/v1/investigations/:id`. Renders:
- Investigation ID and start timestamp at the top.
- Screenshot via `<img src="/api/v1/investigations/:id/screenshot" />`. If the `<img>` fires `onError`, replaces it with a "Screenshot unavailable" placeholder — no broken image icon.
- If `synthesis` is present: five claim blocks, each showing the claim label, value, a confidence badge (`low` / `medium` / `high` with distinct colours), and the evidence text (visible by default, not behind a toggle).
- If `synthesis` is absent: the `hypothesis` object rendered and labelled "Preliminary hypothesis — synthesis not yet available."

Create `pages/InvestigationPage.tsx` and wire `/investigations/:id` to it.

**Verified when:** Navigating to `/investigations/<id>` for a complete investigation shows the screenshot (or the placeholder), all five synthesis claims with confidence badges and evidence, and the investigation metadata. For an investigation without synthesis, the hypothesis block is shown with the preliminary label.

---

## WU-T15: Implement ProgressStream component

**Satisfies:** WU-2

Create `web/src/components/ProgressStream.tsx`. When rendered for an in-flight investigation (status not `complete` or `failed`):
- Opens an `EventSource` to `/api/v1/investigations/:id/events`.
- Renders incoming events in a log-style list, newest at the bottom:
  - `phase_transition`: bold phase name and timestamp.
  - `tool_call`: tool name and a truncated view of the input.
  - `tool_result`: tool name, summary text, elapsed time since the matching `tool_call`.
  - `log`: plain message.
  - `complete`: final verdict text; closes the `EventSource`.
  - `failed`: error reason in red; closes the `EventSource`.
- On `EventSource` error, displays a reconnecting indicator; the browser's native `EventSource` reconnection handles retry automatically.
- Closes the `EventSource` on component unmount.

Integrate `ProgressStream` into `InvestigationPage.tsx`: show it when `status` is not terminal, hide it (or show "Investigation complete") once a terminal event arrives and trigger a re-fetch of the investigation to populate `ReportView`.

**Verified when:** Submitting a URL and being redirected to `/investigations/:id` shows the `ProgressStream` component emitting phase and tool events as the pipeline runs. After the `complete` event, the report view populates with synthesis data without a manual page refresh.

---

## WU-T16: Embed web/dist into Go binary

**Satisfies:** WU-1, WU-7

In `internal/api/static.go`, add:

```go
//go:embed web/dist
var staticFS embed.FS
```

The embed path must be relative to the module root; adjust the embed directive path to match the actual layout (`../../web/dist` will not work — the Go embed path must be relative to the file containing the directive, which may require moving the embed declaration to a file in the module root or creating a thin `web/web.go` package at module root level with the embed directive). Resolve the correct location before implementing.

Register a handler on the ServeMux in `internal/api/routes.go` that:
- Serves files from the embedded FS for requests that match an existing file.
- Returns `index.html` for all other unmatched paths (SPA fallback) so that deep links to `/investigations/:id` work without a 404.

**Verified when:** `go build ./cmd/server/` produces a binary (deleted after the check) that, when run, serves the React app at `http://localhost:8080/` without needing `web/dist/` on disk. `curl http://localhost:8080/some/unknown/path` returns the `index.html` content, not a 404.

---

## WU-T17: Update docs/ for web-ui additions

**Satisfies:** WU-7 (API versioning visible in architecture), WU-1 through WU-6 (new surface)

Per the working agreement in CLAUDE.md, update the three diagrams before the feature is considered complete:
- `docs/architecture.md` — add `cmd/server`, `internal/api`, and `web/` to the package diagram. Add the HTTP server + SSE broker to the pipeline diagram showing the browser as a new consumer of the same pipeline.
- `docs/data-model.md` — no schema changes in this feature; confirm no update needed and note it.
- `docs/egress-proxy.md` — confirm the proxy topology is unchanged (the web server calls the same containerised fetcher); note it in the doc if not already clear.

**Verified when:** The three docs files are updated and `git diff docs/` shows no untracked changes after this task. The architecture diagram reflects `cmd/server` and `internal/api` as new nodes.

---
