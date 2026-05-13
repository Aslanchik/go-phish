package pipeline

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/agent"
	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
	"github.com/aslanchik/go-phish/internal/report"
	"github.com/aslanchik/go-phish/internal/synthesis"
	"github.com/aslanchik/go-phish/internal/tools"
)

// Event is a progress update emitted during a pipeline run.
type Event struct {
	InvestigationID string
	Type            string // phase_transition | tool_call | tool_result | log | complete | failed
	Timestamp       time.Time
	Data            map[string]any
}

// Run executes phases 1–4 for an investigation that has already been created in
// the database. progress is called for each event; pass nil to run silently.
// skipLLM bypasses all LLM calls and uses stub outputs (for testing).
func Run(
	ctx context.Context,
	invID string,
	conn *sql.DB,
	llmClient *anthropic.Client,
	skipLLM bool,
	progress func(Event),
) error {
	emit := func(typ string, data map[string]any) {
		if progress == nil {
			return
		}
		progress(Event{
			InvestigationID: invID,
			Type:            typ,
			Timestamp:       time.Now().UTC(),
			Data:            data,
		})
	}

	fail := func(msg string, args ...any) error {
		err := fmt.Errorf(msg, args...)
		_ = db.UpdateStatus(ctx, conn, invID, db.StatusFailed, err.Error())
		emit("failed", map[string]any{"reason": err.Error()})
		return err
	}

	// --- Phase 1: fetch ---
	if err := db.UpdateStatus(ctx, conn, invID, db.StatusFetching, ""); err != nil {
		return fail("update status fetching: %w", err)
	}
	emit("phase_transition", map[string]any{"phase": "fetching"})

	inv, err := db.GetInvestigation(ctx, conn, invID)
	if err != nil {
		return fail("read investigation: %w", err)
	}

	log.Printf("phase 1: fetching %s", inv.URL)
	result, err := fetcher.Run(ctx, inv.URL)
	if err != nil {
		return fail("fetch: %w", err)
	}
	if err := db.UpdateArtifacts(ctx, conn, invID, result); err != nil {
		return fail("store artifacts: %w", err)
	}

	// --- Phase 2: hypothesis ---
	if err := db.UpdateStatus(ctx, conn, invID, db.StatusHypothesizing, ""); err != nil {
		return fail("update status hypothesizing: %w", err)
	}
	emit("phase_transition", map[string]any{"phase": "hypothesizing"})

	log.Printf("phase 2: generating hypothesis")
	var hyp hypothesis.Hypothesis
	if skipLLM {
		hyp = hypothesis.Hypothesis{
			Brand:          "unknown (--skip-llm)",
			TargetedAction: "other",
			Confidence:     "low",
			Reasoning:      "LLM call skipped; no analysis performed.",
		}
	} else {
		screenshotBytes, err := base64.StdEncoding.DecodeString(result.Screenshot)
		if err != nil {
			return fail("decode screenshot: %w", err)
		}
		hyp, err = hypothesis.Generate(ctx, llmClient, screenshotBytes, result.RenderedDOM)
		if err != nil {
			return fail("hypothesis: %w", err)
		}
	}
	if err := db.UpdateHypothesis(ctx, conn, invID, hyp); err != nil {
		return fail("store hypothesis: %w", err)
	}

	// --- Phase 3: enrichment ---
	if err := db.UpdateStatus(ctx, conn, invID, db.StatusEnriching, ""); err != nil {
		return fail("update status enriching: %w", err)
	}
	emit("phase_transition", map[string]any{"phase": "enriching"})

	inv, err = db.GetInvestigation(ctx, conn, invID)
	if err != nil {
		return fail("read investigation before enrichment: %w", err)
	}

	if !skipLLM {
		log.Printf("phase 3: starting enrichment agent loop")

		toolServer, err := tools.New(ctx, llmClient)
		if err != nil {
			return fail("start tool server: %w", err)
		}
		defer toolServer.Stop()

		toolCB := func(toolName string, input, output json.RawMessage) {
			if output == nil {
				emit("tool_call", map[string]any{"tool": toolName, "input": json.RawMessage(input)})
			} else {
				summary := string(output)
				if len(summary) > 200 {
					summary = summary[:200]
				}
				emit("tool_result", map[string]any{"tool": toolName, "summary": summary})
			}
		}

		enrichTrace, enrichSummary, err := agent.Run(ctx, inv, llmClient, toolServer.Client, toolCB)
		if err != nil {
			return fail("enrichment: %w", err)
		}
		log.Printf("phase 3: complete — %d tool calls", len(enrichTrace))

		traceJSON, err := json.Marshal(enrichTrace)
		if err != nil {
			return fail("marshal enrichment trace: %w", err)
		}
		if err := db.UpdateEnrichment(ctx, conn, invID, traceJSON, enrichSummary); err != nil {
			return fail("store enrichment: %w", err)
		}
	}

	// --- Phase 4: synthesis ---
	if err := db.UpdateStatus(ctx, conn, invID, db.StatusSynthesizing, ""); err != nil {
		return fail("update status synthesizing: %w", err)
	}
	emit("phase_transition", map[string]any{"phase": "synthesizing"})

	log.Printf("phase 4: synthesising findings")
	inv, err = db.GetInvestigation(ctx, conn, invID)
	if err != nil {
		return fail("read investigation before synthesis: %w", err)
	}

	var synthResult synthesis.Result
	if skipLLM {
		skipped := synthesis.Claim{Confidence: "low", Evidence: []string{"LLM call skipped; no analysis performed"}}
		synthResult = synthesis.Result{
			BrandImpersonated:   skipped,
			KitIdentification:   skipped,
			ExfilTarget:         skipped,
			InfrastructureNotes: skipped,
			Verdict:             synthesis.Claim{Value: "inconclusive", Confidence: "low", Evidence: []string{"LLM call skipped; no analysis performed"}},
		}
	} else {
		synthResult, err = synthesis.Generate(ctx, llmClient, inv)
		if err != nil {
			return fail("synthesis: %w", err)
		}
	}

	synthJSON, err := json.Marshal(synthResult)
	if err != nil {
		return fail("marshal synthesis result: %w", err)
	}
	if err := db.UpdateSynthesis(ctx, conn, invID, synthJSON); err != nil {
		return fail("store synthesis: %w", err)
	}

	if err := db.UpdateStatus(ctx, conn, invID, db.StatusComplete, ""); err != nil {
		return fail("update status complete: %w", err)
	}

	inv, err = db.GetInvestigation(ctx, conn, invID)
	if err != nil {
		return fail("read investigation for report: %w", err)
	}

	reportText := report.Format(inv)
	if err := db.UpdateReport(ctx, conn, invID, reportText); err != nil {
		return fail("store report: %w", err)
	}

	emit("complete", map[string]any{"verdict": synthResult.Verdict.Value})
	return nil
}
