# enrichment-tools: Requirements

## Overview

Phase 3 extends the pipeline with a targeted enrichment loop. After the Phase 2 hypothesis, an LLM agent receives the hypothesis and a set of OSINT tools and decides what to investigate and in what order. Each tool call produces structured evidence. The loop ends when the agent signals it is done or a cap is reached. All evidence is persisted and feeds Phase 4 synthesis.

The tools are exposed via an MCP (Model Context Protocol) server written in Go, running in-process alongside the main pipeline. This is the primary learning objective: writing a real agentic loop by hand — no framework, tool-selection logic owned by the model, failure modes visible and documented.

---

## ET-1: Agent tool loop

The enrichment phase is an agentic loop: the model receives the Phase 2 hypothesis and a set of tools, calls them in whatever order it judges useful, and signals completion when it is done.

**Acceptance criteria:**
- The loop begins with the Phase 2 hypothesis and the full tool list available
- The model may call any tool zero or more times per turn
- The loop terminates when the model sends a final text response (no tool calls) or a hard iteration cap is reached
- The iteration cap is configurable, not hardcoded; a reasonable default is defined in design.md
- Every tool call and its result is recorded; the full call trace is stored in Postgres before Phase 4 begins
- If the model calls a tool with invalid arguments, the error is returned to the model as a tool result — the loop does not abort
- If a tool call fails due to an external service being unavailable, the error is returned to the model as a tool result — the loop does not abort

---

## ET-2: MCP tool server

The enrichment tools are served via an MCP server implemented in Go and running in-process. The server is the authoritative boundary between the agent loop and external services.

**Acceptance criteria:**
- The MCP server is implemented in Go as part of this repository — no third-party MCP server
- The server runs in-process (not as a separate daemon or subprocess)
- Each tool has a defined input schema and a defined output schema; both are documented in design.md
- Tool implementations are independently testable without running the full pipeline

---

## ET-3: whois_lookup tool

Look up registration information for a domain.

**Acceptance criteria:**
- Input: `domain` (string)
- Output: registrar, registration date, expiry date, registrant org (if public), raw whois text
- Registration date is surfaced as a top-level field — it is the highest-signal field for freshly-registered phishing domains
- If the domain has no whois record or the query times out, a structured error is returned (not a Go error — a tool result the model can reason about)

---

## ET-4: cert_transparency tool

Query certificate transparency logs for a domain via crt.sh.

**Acceptance criteria:**
- Input: `domain` (string)
- Output: list of certificates — each with `common_name`, `san_entries`, `issuer`, `not_before`, `not_after`
- Results are capped at a configurable maximum to avoid overwhelming the context window
- If crt.sh is unreachable or returns no results, a structured result (empty list or error message) is returned

---

## ET-5: urlscan_lookup tool

Query urlscan.io for prior scans of a URL.

**Acceptance criteria:**
- Input: `url` (string)
- Output: list of prior scans — each with `scan_date`, `verdict` (malicious/suspicious/benign), `tags`, `screenshot_url`, `dom_url`
- If no prior scans exist, an empty list is returned
- If the urlscan.io API is unreachable, a structured error is returned
- API key is read from an environment variable; if not set, the tool returns a structured error rather than panicking

---

## ET-6: urlhaus_check tool

Check a URL or domain against URLhaus for known malware/phishing associations.

**Acceptance criteria:**
- Input: `url_or_domain` (string)
- Output: `found` (bool), `threat_type`, `tags`, `date_added`, `urls_on_host` (list, capped)
- If the entry is not found, `found: false` is returned — not an error
- No API key required (URLhaus has a public API)

---

## ET-7: analyze_js tool

Analyse a JavaScript file for phishing kit indicators.

**Acceptance criteria:**
- Input: `js_content` (string) — the raw JS source
- Output: structured findings — `kit_name` (if identifiable), `exfil_urls` (list of URLs the script posts data to), `obfuscation_detected` (bool), `notable_strings` (list), `summary` (one paragraph)
- This is an LLM call, not a regex scan; the same Anthropic client used for the main pipeline is reused
- If `js_content` exceeds a configurable token budget, it is truncated with a note in the output
- The tool is callable with any JS content — it is not restricted to JS collected during Phase 1

---

## ET-8: Enrichment persistence

All evidence collected during Phase 3 is stored before Phase 4 begins.

**Acceptance criteria:**
- The `investigations` table is extended to store the full enrichment call trace: every tool name, input arguments, and result, in call order
- The schema change is a new migration — it does not alter the existing columns
- If Postgres is unavailable when Phase 3 tries to persist, the pipeline fails with a descriptive error; partial enrichment results are not silently discarded

---

## Safety requirements

- **S-5:** The enrichment tools make read-only queries to external services — no write operations, no form submissions, no registrations
- **S-6:** Tool results are passed back to the model as context — the model never receives raw credentials, API keys, or internal infrastructure details from tool outputs
- **S-7:** The `analyze_js` tool sends JS content to the Anthropic API; operators must be aware that phishing kit source code will leave the local environment. This is documented in the CLI help text and in design.md — it is not a hidden behaviour.

---

## Out of scope

- `compare_to_brand_login` (vision-based brand comparison) — deferred to a later iteration
- `analyze_form` — deferred; form data is already captured in Phase 1 and available to the model as context
- `virustotal_lookup` — deferred; API key management and rate limits add friction without meaningfully changing the learning objectives
- Phase 4 synthesis (this spec covers Phase 3 only)
- Any UI for reviewing enrichment results
- Caching tool results across investigations
