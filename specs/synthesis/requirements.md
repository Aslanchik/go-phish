# synthesis: Requirements

## Overview

Phase 4 closes the pipeline. After Phase 3 enrichment, an LLM receives the collected evidence and produces a final structured verdict. Every factual claim in the verdict carries its own confidence level and supporting evidence citation — this is the primary mechanism for exposing hallucinations. The result is persisted and replaces the current Phase 2–only report.

---

## SP-1: Synthesis LLM call

An LLM call takes the accumulated evidence from Phases 1–3 and produces a structured verdict.

**Acceptance criteria:**
- Input to the LLM: Phase 2 hypothesis, Phase 3 enrichment call trace (tool names, inputs, outputs in call order), Phase 1 artifact summary (final URL, form actions, JS file count)
- Raw DOM and screenshot are **not** re-sent in the synthesis call — the model reasons from collected evidence, not raw page content
- Output is structured — schema defined in design.md
- If the LLM call fails or returns a malformed response, the investigation fails with a descriptive error; no retry in v1

---

## SP-2: Per-claim confidence levels

Every factual claim in the synthesis output has an associated confidence level and evidence citation. This is the primary anti-hallucination mechanism and the most important requirement in this spec.

**Acceptance criteria:**
- The following claims are each independently assessed:
  - `brand_impersonated` — the brand or organisation being targeted
  - `kit_identification` — whether a known phishing kit is identified (with name if identifiable, empty if not)
  - `exfil_target` — the URL or domain where collected credentials are sent; empty if not determinable
  - `infrastructure_notes` — observations about the domain (registration age, cert issuer, related hosts, etc.)
  - `verdict` — overall assessment: one of `phishing | benign | inconclusive`
- Each claim carries:
  - `confidence`: `low | medium | high`
  - `evidence`: a non-empty string that names the specific source (tool name or artifact) and the observation — e.g. `"whois_lookup: domain registered 3 days ago"` or `"analyze_js: exfil POST to collect.evil.com found in kit source"`. Vague reasoning without a source citation is not acceptable.
- A global confidence score must not replace per-claim confidences; both may co-exist but the per-claim fields are the unit of evaluation
- Both `confidence` and `evidence` are enforced in the output schema — the model cannot omit either

---

## SP-3: Synthesis persistence

The synthesis result is stored in Postgres before the report is printed.

**Acceptance criteria:**
- The `investigations` table gains a `synthesis JSONB` column
- Schema change is a new migration (`0004_add_synthesis.sql`) — it does not alter existing columns
- Synthesis is stored before `status` is updated to `complete`
- If Postgres is unavailable when synthesis tries to persist, the pipeline fails with a descriptive error; the result is not silently discarded

---

## SP-4: Report update

The report printed to stdout reflects the synthesis findings, not just the Phase 2 hypothesis.

**Acceptance criteria:**
- Report includes all five synthesis claims: brand, kit identification, exfil target, infrastructure notes, verdict
- Each claim's confidence level is shown alongside the claim
- Investigation ID and timestamp are present
- Plain text is sufficient — no machine-readable format required
- The stored report in Postgres (`report TEXT`) is identical to what is printed
- `report.Format()` is updated to use synthesis output when available; it falls back to hypothesis-only output if synthesis is absent (for investigations that predate this feature)

---

## SP-5: Pipeline wiring

The synthesis phase is inserted into the pipeline between enrichment and report printing.

**Acceptance criteria:**
- A new status value `synthesizing` exists and is set before the LLM call begins
- `main.go` calls synthesis after enrichment is persisted and before formatting the report
- Under `--skip-llm`, a stub synthesis result is used (same pattern as the Phase 2 stub hypothesis) so the flag continues to allow full pipeline testing without an API key
- Any failure in synthesis updates status to `failed` with a descriptive error message and exits non-zero

---

## Out of scope

- Eval harness execution — `eval_labels` is populated manually, not automatically
- Multi-turn synthesis (single LLM call only)
- Structured diff between hypothesis and synthesis (how the model changed its mind across phases)
- Any UI for reviewing synthesis output
