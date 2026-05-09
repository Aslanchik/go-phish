# synthesis: Tasks

Tasks are ordered. Each must be complete and verifiable before the next begins. If a task surfaces a spec problem, stop — update the relevant spec, re-review, then continue.

Status: `[ ]` todo · `[x]` done · `[~]` in progress

---

## [ ] SP-T1: Migration 0004 — synthesis column

**Satisfies:** SP-3

Write `internal/db/migrations/0004_add_synthesis.sql`:

```sql
-- +goose Up
ALTER TABLE investigations
    ADD COLUMN synthesis JSONB;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN synthesis;
```

**Verified when:** running migrations against a local Postgres instance adds the `synthesis` column to `investigations`; rolling back removes it cleanly.

---

## [ ] SP-T2: db — synthesizing status + UpdateSynthesis

**Satisfies:** SP-3, SP-5

- Add `StatusSynthesizing Status = "synthesizing"` to `internal/db/status.go`
- Add to `internal/db/investigations.go`:

```go
func UpdateSynthesis(ctx context.Context, conn *sql.DB, id string, result json.RawMessage) error
```

Writes `result` to the `synthesis` column on the matching row.

- Add `Synthesis json.RawMessage` field to the `Investigation` struct
- Update `GetInvestigation` to scan the new column

**Verified when:** `UpdateSynthesis` sets the column on an existing row; `GetInvestigation` returns it correctly; a nil/empty `json.RawMessage` is stored as SQL NULL without error.

---

## [ ] SP-T3: synthesis.Generate

**Satisfies:** SP-1, SP-2

Create `internal/synthesis/synthesis.go`.

### Types

```go
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
```

### Function signature

```go
func Generate(ctx context.Context, client *anthropic.Client, inv db.Investigation) (Result, error)
```

### Implementation

Construct a single user message with three text blocks (in order):

1. **Hypothesis block** — the raw `inv.Hypothesis` JSON, labelled `## Phase 2 Hypothesis`
2. **Enrichment trace block** — the raw `inv.EnrichmentTrace` JSON, labelled `## Phase 3 Enrichment Evidence`. If `inv.EnrichmentTrace` is empty or null, use the label with an empty array `[]` so the model knows no tools ran.
3. **Artifact summary block** — labelled `## Page Artifacts`, deterministically constructed from `inv.FinalURL`, `inv.Forms` (list of action URLs), and the count of entries in `inv.JSFiles`. No raw DOM. No screenshot.

Define the `record_synthesis` tool with the flat JSON Schema from design.md (five explicit claim objects; no `$ref`). Set `tool_choice` to force the model to call `record_synthesis`.

Parse the tool_use block from the response into a `Result`. If the response contains no tool_use block, return a typed error — do not silently return a zero value.

Return an error (not a stub result) if `inv.Hypothesis` is null — synthesis without a prior hypothesis is not valid.

Use model `claude-sonnet-4-6`.

### Tests

Write `internal/synthesis/synthesis_test.go` with at least:
- A test that a well-formed tool_use response unmarshals into a `Result` with all five claims populated
- A test that a response with no tool_use block returns an error
- A test that a null hypothesis returns an error before any API call is made

**Verified when:** `go test ./internal/synthesis/` passes; `go vet ./internal/synthesis/` passes.

---

## [ ] SP-T4: Update report.Format

**Satisfies:** SP-4

Update `internal/report/report.go`:

- If `inv.Synthesis` is non-empty, unmarshal it into a `synthesis.Result` and render the synthesis section first, followed by the Phase 2 hypothesis as a reference section
- If `inv.Synthesis` is empty or null, render hypothesis-only (current behaviour — fallback for pre-synthesis investigations)

**Synthesis section format** (match exactly):

```
--- Synthesis ---

Verdict:              <value>  [<confidence>]
  evidence: <evidence>

Brand impersonated:   <value>  [<confidence>]
  evidence: <evidence>

Kit identified:       <value>  [<confidence>]
  evidence: <evidence>

Exfil target:         <value>  [<confidence>]
  evidence: <evidence>

Infrastructure:       <value>  [<confidence>]
  evidence: <evidence>
```

Followed by:

```
--- Phase 2 Hypothesis (for reference) ---

Brand:                <brand>
Targeted action:      <targeted_action>
Confidence:           <confidence>
Reasoning:            <reasoning>
```

Label column width: 22 characters (consistent with existing `%-20s` pattern — adjust to fit new labels).

**Verified when:** given a hand-constructed `Investigation` with a populated `Synthesis` field, `Format()` returns a string containing all five synthesis claims each with a `[confidence]` suffix and an `evidence:` line; given an `Investigation` with no `Synthesis`, output is unchanged from today.

---

## [ ] SP-T5: Wire synthesis into main pipeline

**Satisfies:** SP-5

In `cmd/gophish/main.go`, after `db.UpdateEnrichment` and before `report.Format`:

1. `db.UpdateStatus(ctx, conn, inv.ID, db.StatusSynthesizing, "")`
2. `inv, err = db.GetInvestigation(ctx, conn, inv.ID)` — refresh so synthesis sees enrichment columns
3. If `!skipLLM`: call `synthesis.Generate(ctx, &llmClient, inv)`; on error: call `fail(...)`
4. If `skipLLM`: use stub `synthesis.Result` — all claims with `Confidence: "low"`, `Evidence: "LLM call skipped; no analysis performed"`, `Verdict.Value: "inconclusive"`
5. Marshal result to `json.RawMessage`; call `db.UpdateSynthesis`; on error: call `fail(...)`
6. `db.UpdateStatus(ctx, conn, inv.ID, db.StatusComplete, "")`
7. `inv, err = db.GetInvestigation(ctx, conn, inv.ID)` — refresh so report sees synthesis column

Add `log.Printf("phase 4: synthesising findings")` before step 1, consistent with existing phase log lines.

**Verified when:** `gophish <url>` (all env vars set) runs to completion; `status` is `complete` in Postgres; `synthesis` column is non-null JSON; report printed to stdout contains the synthesis section with per-claim confidence levels.

---

## [ ] SP-T6: End-to-end smoke test

**Satisfies:** all requirements

Run `gophish <url>` against a known phishing URL (reuse one from the Phase 3 smoke test).

Verify:
- Exit code 0
- `synthesis` column in Postgres is non-null and contains all five claims
- Each claim has a non-empty `evidence` string
- Report printed to stdout includes the synthesis section with `[confidence]` on each line
- `status` is `complete`
- Observations (tool ordering, verdict accuracy, evidence quality) written to `agent-notes.md`

**Verified when:** all checks pass and `agent-notes.md` is updated.
