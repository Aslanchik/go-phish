# synthesis: Design

## Overview

Phase 4 receives the accumulated evidence from Phases 1–3 and makes a single LLM call to produce a structured verdict. Every claim has a confidence level and a source-cited evidence string. The result is stored as `synthesis JSONB` on the investigation row; the existing `report TEXT` column is updated to reflect synthesis findings.

---

## Package structure

The existing stub grows into a real implementation:

```
internal/synthesis/
    synthesis.go    — Generate() and all types
```

`cmd/gophish/main.go` gains one new step between enrichment persistence and report formatting.

No other packages change except `internal/db/` (new migration + CRUD function + Investigation struct field) and `internal/report/` (Format updated to use synthesis when present).

---

## Structured output via tool_use

**Decision:** same pattern as Phase 2 — force structured output via a single `record_synthesis` tool. The model is given one tool and instructed to call it. The host reads the tool_use block and unmarshals the arguments.

If the response contains no tool_use block, the investigation fails with an explicit protocol error. No retry in v1.

### `record_synthesis` input schema

Each of the five claims is written out explicitly as a full object — no `$ref` or `$defs`. Anthropic's tool input schema support for JSON Schema is documented as a subset; `$ref` resolution is not guaranteed, so the flat form is used to avoid undefined behaviour.

```json
{
  "type": "object",
  "required": ["brand_impersonated", "kit_identification", "exfil_target", "infrastructure_notes", "verdict"],
  "properties": {
    "brand_impersonated": {
      "type": "object",
      "required": ["value", "confidence", "evidence"],
      "properties": {
        "value":      { "type": "string" },
        "confidence": { "type": "string", "enum": ["low", "medium", "high"] },
        "evidence":   { "type": "string", "minLength": 1 }
      }
    },
    "kit_identification": {
      "type": "object",
      "required": ["value", "confidence", "evidence"],
      "properties": {
        "value":      { "type": "string" },
        "confidence": { "type": "string", "enum": ["low", "medium", "high"] },
        "evidence":   { "type": "string", "minLength": 1 }
      }
    },
    "exfil_target": {
      "type": "object",
      "required": ["value", "confidence", "evidence"],
      "properties": {
        "value":      { "type": "string" },
        "confidence": { "type": "string", "enum": ["low", "medium", "high"] },
        "evidence":   { "type": "string", "minLength": 1 }
      }
    },
    "infrastructure_notes": {
      "type": "object",
      "required": ["value", "confidence", "evidence"],
      "properties": {
        "value":      { "type": "string" },
        "confidence": { "type": "string", "enum": ["low", "medium", "high"] },
        "evidence":   { "type": "string", "minLength": 1 }
      }
    },
    "verdict": {
      "type": "object",
      "required": ["value", "confidence", "evidence"],
      "properties": {
        "value":      { "type": "string", "enum": ["phishing", "benign", "inconclusive"] },
        "confidence": { "type": "string", "enum": ["low", "medium", "high"] },
        "evidence":   { "type": "string", "minLength": 1 }
      }
    }
  }
}
```

The `evidence` field has `minLength: 1` in the schema. This enforces the source-citation requirement at the protocol level — the model cannot return an empty string.

---

## Go types

```go
// internal/synthesis/synthesis.go

type Claim struct {
    Value      string `json:"value"`
    Confidence string `json:"confidence"`
    Evidence   string `json:"evidence"`
}

type Result struct {
    BrandImpersonated   Claim `json:"brand_impersonated"`
    KitIdentification   Claim `json:"kit_identification"`
    ExfilTarget         Claim `json:"exfil_target"`
    InfrastructureNotes Claim `json:"infrastructure_notes"`
    Verdict             Claim `json:"verdict"`
}

func Generate(ctx context.Context, client *anthropic.Client, inv db.Investigation) (Result, error)
```

`Generate` is the only exported symbol. All schema construction and prompt assembly are unexported.

---

## LLM input construction

The synthesis call receives three text blocks in a single user message:

**Block 1 — Phase 2 hypothesis:**
The raw hypothesis JSON from `inv.Hypothesis`. Four fields: `brand`, `targeted_action`, `confidence`, `reasoning`.

**Block 2 — Phase 3 enrichment trace:**
The raw `inv.EnrichmentTrace` JSON (the ordered `[]ToolCall` array). Each entry has `tool`, `input`, `output`, `called_at`.

**Decision: full trace, not a summary.** A typical investigation has 5–10 tool calls. Passing the full trace avoids any lossy summarisation and gives the model direct access to every tool output. Context cost is negligible at this scale.

**Block 3 — Phase 1 artifact summary:**
A short, deterministically constructed text block:
- Final URL (from `inv.FinalURL`)
- Form actions: list of `{action, method, fields}` objects from `inv.Forms`
- JS files loaded: count from `inv.JSFiles`

Raw DOM and screenshot are **not included** in the synthesis call. Phase 2 already extracted the visual hypothesis; synthesis reasons from collected evidence, not raw page content. Including the screenshot would re-run Phase 2 inside Phase 4 and risk over-weighting visual similarity over what the tools actually found.

---

## Postgres changes

### New migration: `0004_add_synthesis.sql`

```sql
-- +goose Up
ALTER TABLE investigations
    ADD COLUMN synthesis JSONB;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN synthesis;
```

### Investigation struct

Add `Synthesis json.RawMessage` to `db.Investigation`. Update `GetInvestigation` to scan the new column.

### New CRUD function

```go
// internal/db/investigations.go
func UpdateSynthesis(ctx context.Context, conn *sql.DB, id string, result json.RawMessage) error
```

Takes pre-marshalled JSON — the caller (`main.go`) marshals the `synthesis.Result` before passing it in. This matches the pattern used by `UpdateEnrichment` and keeps `internal/db` free of any import of `internal/synthesis`. The two packages would otherwise create a cycle: `synthesis` imports `db` (for `db.Investigation`), so `db` cannot import `synthesis`.

### New status value: `synthesizing`

```go
StatusSynthesizing Status = "synthesizing"
```

Status transition:

```
pending → fetching → hypothesizing → enriching → synthesizing → complete
                                                             ↘ failed
```

---

## Pipeline wiring

In `cmd/gophish/main.go`, after `UpdateEnrichment` and before `report.Format`:

```
log.Printf("phase 4: synthesising findings")
db.UpdateStatus(ctx, conn, inv.ID, db.StatusSynthesizing, "")
inv = db.GetInvestigation(...)      // refresh — needs enrichment columns
synth = synthesis.Generate(ctx, &llmClient, inv)
db.UpdateSynthesis(ctx, conn, inv.ID, synth)
db.UpdateStatus(ctx, conn, inv.ID, db.StatusComplete, "")
inv = db.GetInvestigation(...)      // refresh — needs synthesis column
report.Format(inv)
```

Under `--skip-llm`, a stub `synthesis.Result` is used with all claims set to `Confidence: "low"`, `Evidence: "LLM call skipped; no analysis performed"`, and `Verdict.Value: "inconclusive"`.

---

## Report format

`report.Format()` checks whether `inv.Synthesis` is non-empty. If present, it renders the synthesis section first, followed by the Phase 2 hypothesis for reference. If absent (investigations predating this feature), it renders hypothesis-only as today.

**New format (synthesis present):**

```
=== Phishing Investigation Report ===

Investigation ID:     <id>
Timestamp:            <timestamp>
URL:                  <url>
Final URL:            <url>           ← only if different

--- Synthesis ---

Verdict:              phishing  [high]
  evidence: whois_lookup: domain registered 2 days ago; urlhaus_check: known phishing host

Brand impersonated:   Ledger  [high]
  evidence: analyze_js: Ledger branding in kit source; phase 2 hypothesis confirms

Kit identified:       (unknown)  [low]
  evidence: analyze_js: no recognisable kit signature found

Exfil target:         collect.evil.com  [medium]
  evidence: analyze_js: POST to https://collect.evil.com/collect.php

Infrastructure:       Domain 2 days old; Let's Encrypt cert; no prior scans  [medium]
  evidence: whois_lookup: registered 2026-05-07; cert_transparency: LE issuer; urlscan_lookup: no results

--- Phase 2 Hypothesis (for reference) ---

Brand:                Ledger
Targeted action:      credential_theft
Confidence:           high
Reasoning:            <reasoning text>
```

Confidence levels are rendered inline after the value in brackets: `[high]`, `[medium]`, `[low]`. No padding tricks — plain `fmt.Fprintf` with a fixed label width.

---

## Failure modes

| Failure | Behaviour |
|---|---|
| Synthesis LLM call fails / rate-limited | Investigation fails; status → failed with error message |
| Model does not call `record_synthesis` | Investigation fails with explicit protocol error |
| DB unavailable when persisting synthesis | Pipeline fails with descriptive error; synthesis result discarded |
| `inv.EnrichmentTrace` is empty (no tool calls ran) | Synthesis proceeds with hypothesis + artifact summary only; expected low-confidence output |
| `inv.Hypothesis` is null | `Generate` returns an error — synthesis without a prior hypothesis is not valid |

---

## Open decisions resolved here

**Full trace vs. summary:** full trace (see LLM input section).

**Nested vs. flat claim schema:** nested (`Claim` struct per finding) — cleaner to render in the report and more natural for the model to fill out.

**Screenshot in synthesis input:** excluded — Phase 2 already extracted visual signal; synthesis reasons from evidence, not raw page content (see LLM input section).

**Report includes Phase 2 hypothesis:** yes, as a reference section. Preserves auditability and makes it easy to see when synthesis diverged from the initial hypothesis.

**`inconclusive` verdict:** included — forces the model to represent genuine uncertainty rather than committing to a binary that isn't supported by evidence.

---

## Out of scope

Anything not listed above. No eval harness changes, no multi-turn synthesis, no UI.
