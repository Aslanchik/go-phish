# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

A CLI tool that takes a suspicious URL and produces a structured phishing investigation report: brand impersonated, kit mechanics, credential exfiltration destination, infrastructure overlap, confidence-rated verdict. The goal is hands-on experience with agentic development patterns — tool-use loops, reliability, evals, and prompt engineering. The tool must be real enough to hit genuine failure modes.

## Development methodology: spec-driven

**Code does not get written until the relevant spec is reviewed and approved.** Specs live in `specs/<feature>/`:

1. `requirements.md` — what and why (user stories, acceptance criteria, no implementation details)
2. `design.md` — how (architecture, data models, interface contracts, tradeoffs explicit)
3. `tasks.md` — ordered, atomic, testable work items; each references the requirement(s) it satisfies

If a task reveals a spec problem, update the spec and re-review before continuing. Do not improvise outside the approved tasks.md.

**First feature to spec: `core-pipeline`** — CLI scaffold, Postgres schema, containerized fetcher (Phase 1), hypothesis generation (Phase 2), report stored in Postgres and printed to stdout. No enrichment tools or synthesis yet.

### What to spec tightly vs. loosely

**Tight** (deterministic infrastructure): CLI surface and flags, Postgres schema and migrations, tool input/output contracts, fetcher container interface, eval harness data model.

**Loose** (agent behavior): Phase prompts (specify interface only, not prompt text — prompts belong in code and get iterated against evals), agent reasoning quality (specified as eval metrics), tool selection logic.

If you find yourself writing a prompt in `design.md`, stop. That belongs in code.

## Architecture: 4-phase agent loop

Each phase has its own prompt, allowed tools, and success criteria — enabling separate evals per phase.

**Phase 1 — Initial fetch (deterministic, no LLM)**
Fetch URL in containerized headless browser. Capture: rendered DOM, full-page screenshot, network request log, final URL after redirects, all JS files, all forms with action URLs.

**Phase 2 — Hypothesis generation**
LLM receives screenshot + DOM summary. Produces: claimed brand, likely targeted action, initial confidence.

**Phase 3 — Targeted enrichment (the real agent loop)**
LLM receives hypothesis + tools and decides what to call and in what order. Tools:
- `whois_lookup(domain)` — registration date is highest-signal field
- `cert_transparency(domain)` — via crt.sh
- `urlscan_lookup(url)`
- `virustotal_lookup(url)`
- `urlhaus_check(url_or_domain)`
- `compare_to_brand_login(screenshot, brand_name)` — Claude vision vs. reference screenshot
- `analyze_js(js_content)` — Claude call for kit identification
- `analyze_form(form_data)` — does form action match claimed brand?

**Phase 4 — Synthesis**
Final report: brand impersonated, kit identification, exfiltration target, infrastructure notes, verdict, **per-claim confidence levels** (not a single global confidence — this is where hallucinations get exposed).

## Stack

- **Language:** Go
- **LLM:** Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`). Use tool-use directly — no agent framework. Write the loop by hand.
- **Headless browser:** Rod (`github.com/go-rod/rod`) or chromedp — decide in `design.md`
- **Database:** PostgreSQL
- **Migrations:** golang-migrate or goose — decide in first `design.md`

Ask before adding dependencies.

## Safety constraints (non-negotiable)

These must appear as explicit acceptance criteria in `requirements.md`, not as nice-to-haves:

- Run the headless browser in a Docker container — no host network or filesystem access
- Restrict container egress to only the URL being investigated
- Never auto-submit forms with realistic-looking data
- Filter source feeds (PhishTank/URLhaus) to well-curated sources

If something feels wrong (a URL category we shouldn't touch, a tool capability that escalates risk) — stop and surface it rather than working around it.

## Eval harness

Don't build evals on day 1, but the `core-pipeline` Postgres schema must anticipate them. Design the `eval_labels` table in the first `design.md` so ground-truth data accumulates from the first investigation.

Metrics to track per phase once the harness exists:
- Phase 2: brand identification accuracy
- Phase 3: did the agent call the relevant tools? (rubric-graded)
- Phase 4: hallucination rate on per-claim confidence — when the agent says "high confidence," how often is it right?

The hallucination metric is the most important and the easiest to skip. Don't skip it.

## Open design decisions (resolve in design.md, not here)

- Structured outputs (JSON schema) vs. free-form parsing
- Handling bad tool ordering — let it fail and learn, or constrain via prompt, or constrain via code?
- Phase 3 iteration cap vs. run until agent signals done
- Handling pages that fail to render, time out, or detect headless browser

When `design.md` covers one of these, explicitly list alternatives considered and why the chosen approach won.

## Working agreements

- **Specs before code, always.** No back-filling specs after the fact.
- **Thin vertical slices.** End-to-end working pipeline before any single component is "done."
- **When the agent does something wrong, understand the failure mode first.** Document it in `agent-notes.md` before changing anything. Half the value of this project is seeing failure modes clearly.
- **Commit after each completed task.** Git history tells the story.
- **Delete compiled binaries after build/test.** Never leave `gophish`, `docker/fetcher/fetcher`, or any other output binary as an untracked file. Remove them immediately after the build or test step that produced them.

## Out of scope for v1

Web UI, batch processing, continuous monitoring, form submission tracing, residential proxy/geo-spoofing, cloud deployment.
