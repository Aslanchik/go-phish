# web-ui: Design

## Overview

A Go HTTP server exposes a versioned REST API and SSE event stream. A React + Tailwind CSS + shadcn/ui frontend (in `web/`) is built to static assets and served by the same Go binary. The existing pipeline packages are called directly — no new process boundary.

---

## Directory layout

```
cmd/
  gophish/          existing CLI entrypoint (unchanged)
  server/           new: HTTP server entrypoint
internal/
  api/              new: HTTP handlers, SSE broker, middleware
web/                new: React app (Vite)
  src/
  dist/             built assets, committed or generated at build time
```

---

## Real-time mechanism: SSE

**Decision:** Server-Sent Events over WebSocket.

**Alternatives considered:**
- WebSocket — bidirectional, but the client never needs to send data after the initial POST. Adds complexity (upgrade handshake, framing, ping/pong) for no benefit here.
- Polling — simpler but wastes connections and adds latency to status transitions.

**Why SSE wins:** Investigations are a one-way stream of status events from server to client. SSE is HTTP/1.1 compatible, works through standard proxies, has native browser reconnection (`EventSource`), and needs zero additional library on either side.

**Reconnection behaviour:** The server assigns a monotonic sequence number to each event for an investigation. On reconnect the client sends `Last-Event-ID`; the server replays any buffered events with higher sequence numbers (buffer held in memory for the lifetime of the investigation, max ~20 events). Events are not persisted to Postgres.

---

## API endpoints

All routes under `/api/v1/`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/investigations` | Submit a URL; starts pipeline; returns `{id, status}` |
| `GET` | `/api/v1/investigations` | List all investigations, ordered by `created_at DESC` |
| `GET` | `/api/v1/investigations/:id` | Full investigation record + synthesis report |
| `GET` | `/api/v1/investigations/:id/events` | SSE stream of phase-transition events |
| `GET` | `/api/v1/investigations/:id/screenshot` | Serves the Phase 1 screenshot as `image/png` |

Static assets and the React SPA are served at `/` by the same binary (`GET /*`).

### Event payload (SSE)

All events share a common envelope:

```json
{
  "id": "evt-seq-number",
  "investigation_id": "uuid",
  "type": "phase_transition | tool_call | tool_result | log | complete | failed",
  "timestamp": "2026-05-10T14:22:00Z",
  "data": { ... }
}
```

**Event types:**

| Type | When emitted | `data` fields |
|------|-------------|---------------|
| `phase_transition` | Investigation moves to a new phase | `phase: fetching \| hypothesizing \| enriching \| synthesizing` |
| `tool_call` | Phase 3 invokes a tool | `tool: string, input: object` |
| `tool_result` | Tool returns | `tool: string, summary: string` (not full raw output — summarised to avoid leaking large blobs to the browser) |
| `log` | Noteworthy event within a phase | `message: string` |
| `complete` | Investigation finished successfully | `verdict: string` |
| `failed` | Investigation failed | `reason: string` |

Phase sequence: `fetching` → `hypothesizing` → `enriching` → `synthesizing` → terminal (`complete` or `failed`)

### Error responses

All errors return `{"error": "human-readable message"}`. Internal errors log the full detail server-side and return a generic `"internal server error"` message to the client. Stack traces and DSN strings never appear in the body.

---

## Frontend routes

| Path | View |
|------|------|
| `/` | History list + URL submission form |
| `/investigations/:id` | Full report view |

React Router (or equivalent) handles client-side navigation. The Go server returns `index.html` for any unmatched path so deep links work.

---

## Frontend components (coarse)

- **SubmitForm** — URL input with client-side validation (must parse as a URL with scheme + host), submit button, loading state
- **InvestigationList** — table/list of past investigations; columns: URL, status/verdict, timestamp
- **ReportView** — fetches investigation by ID; shows screenshot, hypothesis, synthesis claims with per-claim confidence badges, evidence
- **ProgressStream** — connects to SSE stream for an in-flight investigation; renders phase transitions, and within Phase 3 renders each `tool_call` / `tool_result` pair as it arrives (tool name, summarised result, elapsed time)

---

## Screenshot serving

Screenshots are stored as `[]byte` in the `investigations` table (existing `screenshot` column). The `/screenshot` endpoint reads the column and streams the bytes with `Content-Type: image/png`. The browser never fetches from the attacker's domain.

If the column is NULL the endpoint returns 404; the frontend shows a "screenshot unavailable" placeholder.

---

## Server binary

`cmd/server/main.go` starts an HTTP server. It accepts `--addr` (default `:8080`) and reuses the existing `db.Connect()` and pipeline entry points from `internal/`. The pipeline runs in a goroutine per investigation; the SSE broker bridges goroutine → connected clients.

No new router dependency — use `net/http` `ServeMux` (Go 1.22+ method+pattern routing is sufficient).

---

## Build

The frontend build (`npm run build` inside `web/`) outputs to `web/dist/`. The Go server embeds `web/dist/` via `//go:embed` so a single binary is deployable. During development, Vite's dev server proxies `/api/` to the Go server.

---

## Open questions deferred to tasks

- Whether to add `chi` for routing or stay with stdlib `ServeMux` (decide before WU-7 task)
- Exact Postgres query for history list pagination (if list grows large — not required for v1)
