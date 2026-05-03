# core-pipeline: Requirements

## Overview

The core pipeline is the first end-to-end vertical slice of the phishing investigation tool. It takes a single suspicious URL, fetches and renders the page in an isolated environment, generates a hypothesis about the phishing campaign, persists the investigation, and prints a report. Every subsequent feature builds on this foundation.

---

## CP-1: CLI entry point

The tool is invoked from the command line with a single URL argument.

**Acceptance criteria:**
- `gophish <url>` runs an investigation and prints the report to stdout on completion
- Missing or malformed URL argument produces a clear error message and non-zero exit code
- Any phase failure exits non-zero with a descriptive message
- `gophish --help` prints usage

---

## CP-2: Containerized page fetch (Phase 1)

The target URL is fetched and rendered in an isolated container. The Go process on the host orchestrates the container but does not perform the rendering itself.

**Acceptance criteria:**
- Rendering happens inside a Docker container; the Go orchestrator runs on the host
- The container has no access to the host filesystem
- Container egress is restricted to the domain of the URL under investigation — no other outbound connections are permitted from within the container
- The fetch captures: rendered DOM (post-JS execution), full-page screenshot, network request log, final URL after all redirects, all JS file contents loaded by the page, all forms with their field names and action URLs
- If the page fails to render, times out, or the container exits non-zero, the investigation fails with a descriptive error — no silent partial results
- Fetch timeout is configurable, not hardcoded

---

## CP-3: Hypothesis generation (Phase 2)

The fetched artifacts are passed to an LLM, which produces a structured hypothesis about the page.

**Acceptance criteria:**
- LLM input: the screenshot and a DOM summary (not the full raw DOM)
- LLM output: brand being impersonated, likely targeted action (one of: credential theft / payment capture / MFA bypass / other), initial confidence level
- Output is structured — schema defined in design.md
- If the LLM call fails or returns a malformed response, the investigation fails with a descriptive error

---

## CP-4: Investigation persistence

Every investigation is stored in Postgres. Persistence happens before the report is printed; a stored investigation is the authoritative record.

**Acceptance criteria:**
- An `investigations` table stores: URL, fetch timestamp, fetched artifacts (or references to them), Phase 2 hypothesis, report output, investigation status
- An `eval_labels` table exists with columns sufficient for future hand-labeling: `brand_impersonated`, `exfil_destination`, `kit_name` (nullable), `is_actually_phishing` — linked to an investigation by ID
- `eval_labels` is empty at the end of core-pipeline; it is designed now so ground-truth data can accumulate from the first real investigation onward
- Schema is managed by versioned migrations; running migrations is part of the documented setup path
- If the database is unreachable at startup, the tool exits before fetching anything — fetching a page and then failing to store it is not acceptable

---

## CP-5: Report output

The tool prints a human-readable report to stdout once the investigation is stored.

**Acceptance criteria:**
- Report includes: URL investigated, brand hypothesized, targeted action, confidence level, investigation ID, timestamp
- Plain text is sufficient — no machine-readable format required at this stage
- The stored report in Postgres is identical to what is printed

---

## Safety requirements

These are acceptance criteria. Any implementation that does not satisfy all of them is not shippable regardless of other functionality.

- **S-1:** The headless browser runs in a Docker container with no host network mode and no host filesystem mounts
- **S-2:** Container egress is restricted to the target domain — no other outbound connections from within the container
- **S-3:** The tool never submits form data, clicks submit buttons, or interacts with attacker-controlled endpoints beyond the initial page load
- **S-4:** Investigation URLs are provided explicitly by the operator — no automated feed ingestion in this version

---

## Out of scope

- Phase 3 (enrichment tools)
- Phase 4 (synthesis)
- Eval harness execution (eval_labels schema is created but never populated automatically)
- Batch processing
- Any web UI
