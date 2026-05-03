# core-pipeline: Tasks

Tasks are ordered. Each must be complete and verifiable before the next begins. If a task surfaces a spec problem, stop — update the relevant spec, re-review, then continue.

Status: `[ ]` todo · `[x]` done · `[~]` in progress

---

## T-01: Initialize Go module and directory structure

**Satisfies:** foundation for all requirements

- `go mod init github.com/aslanchik/go-phish`
- Create directory tree: `cmd/gophish/`, `internal/fetcher/`, `internal/hypothesis/`, `internal/db/`, `internal/report/`, `internal/enrichment/` (stub), `internal/synthesis/` (stub), `internal/agent/` (stub), `internal/tools/` (stub), `docker/fetcher/`, `migrations/`
- Add `.gitignore` (Go standard: binaries, `vendor/`, env files)
- Add stub `main.go` in `cmd/gophish/` that compiles and exits 0

**Verified when:** `go build ./...` succeeds with no errors

---

## T-02: Postgres migrations — investigations table

**Satisfies:** CP-4

- Add goose as a dependency (`github.com/pressly/goose/v3`)
- Write `migrations/0001_create_investigations.sql` per the schema in design.md
- Embed the migrations directory in `internal/db/` using `embed.FS`
- Write a `db.RunMigrations(db *sql.DB)` function that applies pending migrations via goose

**Verified when:** running migrations against a local Postgres instance creates the `investigations` table with the correct columns and types

---

## T-03: Postgres migrations — eval_labels table

**Satisfies:** CP-4

- Write `migrations/0002_create_eval_labels.sql` per the schema in design.md (foreign key to `investigations.id`)

**Verified when:** running migrations creates `eval_labels` with the correct columns; `investigations` FK constraint is enforced

---

## T-04: Database connection and startup check

**Satisfies:** CP-4 (fail before fetching if DB is unreachable)

- Read `DATABASE_URL` from environment in `internal/db/`; fail with a clear error if unset
- Open connection with `database/sql` + `lib/pq` (or `pgx`)
- Expose `db.Open() (*sql.DB, error)` that pings the DB before returning
- Call `db.Open()` and `db.RunMigrations()` as the first step in `main.go`; exit non-zero if either fails

**Verified when:** pointing at a stopped Postgres instance causes the tool to exit immediately with a descriptive error before any network activity

---

## T-05: Investigation CRUD in db package

**Satisfies:** CP-4

- Define `Investigation` struct in `internal/db/` matching the schema
- Implement:
  - `CreateInvestigation(ctx, db, url string) (Investigation, error)` — inserts with status `pending`, returns the new row
  - `UpdateStatus(ctx, db, id UUID, status string, errMsg string) error`
  - `UpdateArtifacts(ctx, db, id UUID, artifacts FetchResult) error`
  - `UpdateHypothesis(ctx, db, id UUID, hypothesis Hypothesis) error`
  - `UpdateReport(ctx, db, id UUID, report string) error`

**Verified when:** each function can be exercised against a local DB and the correct rows appear; a second call with the same ID updates in place

---

## T-06: Fetcher container binary

**Satisfies:** CP-2

- Write a standalone Go program in `docker/fetcher/main.go` with its own `go.mod`
- Add Rod (`github.com/go-rod/rod`) as a dependency in the fetcher module
- On startup: read `TARGET_URL` and `FETCH_TIMEOUT_SECONDS` from environment; fail if `TARGET_URL` is absent
- Navigate to `TARGET_URL`, wait for network idle or timeout
- Capture: rendered DOM, full-page screenshot (PNG), network request log, final URL after redirects, all JS file contents, all forms with field names/types and action URLs
- Marshal result to the JSON schema in design.md and write to stdout
- Write diagnostic messages to stderr only; stdout must be pure JSON
- Exit 0 on success, 1 on any error

**Verified when:** running the binary locally (outside Docker) against a known URL produces valid JSON on stdout with all six fields populated; errors print to stderr and exit 1

---

## T-07: Fetcher Dockerfile

**Satisfies:** CP-2, S-1

- Write `docker/fetcher/Dockerfile` with a multi-stage build:
  - Build stage: compile `docker/fetcher/main.go` to a static binary
  - Runtime stage: minimal base (e.g. `debian:bookworm-slim`) with Chromium installed
- Apply security posture at runtime: `--cap-drop ALL`, `--security-opt no-new-privileges`, read-only root filesystem with a `tmpfs` mount at `/tmp` (Chromium requires a writable temp dir)
- `ENTRYPOINT` is the compiled fetcher binary

**Verified when:** `docker build` succeeds; `docker run --env TARGET_URL=<url> <image>` produces valid JSON on stdout for a real URL

---

## T-08: Host-side fetcher orchestrator with egress restriction

**Satisfies:** CP-2, S-2

- Implement `internal/fetcher/Run(ctx, url string) (FetchResult, error)`
- Before starting the container:
  - Resolve the target domain to its IP addresses
  - Create a per-investigation Docker bridge network
  - Install iptables OUTPUT rules on the network interface to whitelist only those IPs (plus the Docker internal DNS resolver at 127.0.0.11); drop all other egress
- Start the container with `TARGET_URL` and `FETCH_TIMEOUT_SECONDS` set, attached to that network
- Stream container stdout; on exit code 0, unmarshal JSON into `FetchResult`; on non-zero exit, collect stderr and return an error
- Tear down the network and iptables rules after the container exits (success or failure)

**Verified when:** `fetcher.Run()` against a real URL returns a populated `FetchResult`; a second simultaneous call for a different URL does not share a network; making an outbound request to a non-target domain from inside the container is blocked

---

## T-09: DOM summary extraction

**Satisfies:** CP-3

- Implement `internal/hypothesis/DOMSummary(dom string) Summary` in pure Go (no LLM)
- Extracts from the rendered DOM:
  - `<title>` content
  - `<meta name="description">` content
  - All form fields (names, types, action URLs) — may reuse data from `FetchResult.Forms`
  - Visible text, stripped of HTML tags, truncated to 2000 characters
- Returns a struct that can be serialized to a compact string for the LLM prompt

**Verified when:** given a saved DOM from a known phishing page, the summary contains title, forms, and truncated text; the output is under 2500 characters total

---

## T-10: Hypothesis LLM call

**Satisfies:** CP-3

- Add Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`) to the main module
- Implement `internal/hypothesis/Generate(ctx, client, screenshot []byte, dom string) (Hypothesis, error)`
- Construct the API request:
  - Image content block: base64 screenshot
  - Text content block: DOM summary from T-09
  - Tool definition: `record_hypothesis` with the schema from design.md
  - `tool_choice`: force the model to call `record_hypothesis`
- Parse the tool_use block from the response into a `Hypothesis` struct: `Brand`, `TargetedAction`, `Confidence`, `Reasoning`
- If the response contains no tool_use block, return a typed error
- Read `ANTHROPIC_API_KEY` from environment; fail at startup if unset

**Verified when:** given a screenshot and DOM from a real phishing page, the call returns a populated `Hypothesis` with all four fields; missing `record_hypothesis` in the response returns an error without panicking

---

## T-11: Report formatter

**Satisfies:** CP-5

- Implement `internal/report/Format(inv Investigation) string`
- Plain text output including: URL investigated, brand hypothesized, targeted action, confidence, reasoning, investigation ID, timestamp
- Implement `internal/report/Print(inv Investigation)` that writes to stdout

**Verified when:** given a hand-constructed `Investigation` value, `Format()` returns a non-empty string containing all six required fields

---

## T-12: CLI and pipeline wiring

**Satisfies:** CP-1, CP-2, CP-3, CP-4, CP-5

- Implement `cmd/gophish/main.go`:
  - Parse URL argument with `flag`; print usage and exit 1 if missing or not a valid URL
  - Call `db.Open()` → `db.RunMigrations()` → exit 1 on failure
  - Call `db.CreateInvestigation(url)` → update status to `fetching`
  - Call `fetcher.Run(url)` → on failure: update status to `failed`, store error, exit 1
  - Update investigation with artifacts; update status to `hypothesizing`
  - Call `hypothesis.Generate(screenshot, dom)` → on failure: update status to `failed`, store error, exit 1
  - Update investigation with hypothesis; update status to `complete`
  - Call `report.Format()` → store report text → call `report.Print()`
  - Exit 0

**Verified when:** `gophish --help` prints usage; `gophish` (no args) exits 1 with a message; `gophish not-a-url` exits 1 with a message; `gophish <valid-url>` (DB running, API key set) runs to completion and prints a report

---

## T-13: End-to-end smoke test against real phishes

**Satisfies:** all requirements and safety constraints

- Find 3 active phishing URLs from PhishTank (manually selected, well-curated list only)
- Run `gophish <url>` for each
- For each run verify:
  - Exit code 0
  - Report printed to stdout with all required fields
  - Investigation row present in Postgres with status `complete`
  - Hypothesis contains a plausible brand and non-empty reasoning
  - Docker network and iptables rules are cleaned up after the run
- Record observations in `agent-notes.md`: what the model got right, wrong, or surprising — do not patch the prompt yet

**Verified when:** all three URLs complete without panics or unhandled errors, and the observations are written to `agent-notes.md`
