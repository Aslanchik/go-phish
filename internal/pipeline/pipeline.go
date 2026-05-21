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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aslanchik/go-phish/internal/agent"
	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/fetcher"
	"github.com/aslanchik/go-phish/internal/hypothesis"
	"github.com/aslanchik/go-phish/internal/report"
	"github.com/aslanchik/go-phish/internal/synthesis"
	"github.com/aslanchik/go-phish/internal/telemetry"
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
	fetchCtx, fetchSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.fetch",
		trace.WithAttributes(
			attribute.String(telemetry.AttrPhase, "fetch"),
			attribute.Int(telemetry.AttrPhaseIndex, 1),
		))
	result, err := fetcher.Run(fetchCtx, inv.URL)
	if err != nil {
		fetchSpan.RecordError(err)
		fetchSpan.SetStatus(codes.Error, err.Error())
		fetchSpan.End()
		return fail("fetch: %w", err)
	}
	fetchSpan.End()

	if err := db.UpdateArtifacts(ctx, conn, invID, result); err != nil {
		return fail("store artifacts: %w", err)
	}

	// --- Phase 2: hypothesis ---
	if err := db.UpdateStatus(ctx, conn, invID, db.StatusHypothesizing, ""); err != nil {
		return fail("update status hypothesizing: %w", err)
	}
	emit("phase_transition", map[string]any{"phase": "hypothesizing"})

	log.Printf("phase 2: generating hypothesis")
	hypCtx, hypSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.hypothesis",
		trace.WithAttributes(
			attribute.String(telemetry.AttrPhase, "hypothesis"),
			attribute.Int(telemetry.AttrPhaseIndex, 2),
		))
	var hyp hypothesis.Hypothesis
	if skipLLM {
		hyp = hypothesis.Hypothesis{
			Brand:          "unknown (--skip-llm)",
			TargetedAction: "other",
			Confidence:     "low",
			Reasoning:      "LLM call skipped; no analysis performed.",
		}
	} else {
		screenshotBytes, decErr := base64.StdEncoding.DecodeString(result.Screenshot)
		if decErr != nil {
			hypSpan.RecordError(decErr)
			hypSpan.SetStatus(codes.Error, decErr.Error())
			hypSpan.End()
			return fail("decode screenshot: %w", decErr)
		}
		var hypErr error
		hyp, hypErr = hypothesis.Generate(hypCtx, llmClient, screenshotBytes, result.RenderedDOM)
		if hypErr != nil {
			hypSpan.RecordError(hypErr)
			hypSpan.SetStatus(codes.Error, hypErr.Error())
			hypSpan.End()
			return fail("hypothesis: %w", hypErr)
		}
	}
	if hypJSON, merr := json.Marshal(hyp); merr == nil {
		v, trunc := telemetry.Truncate(string(hypJSON))
		hypSpan.SetAttributes(attribute.String(telemetry.AttrPhaseOutcome, v))
		if trunc {
			hypSpan.SetAttributes(attribute.Bool(telemetry.AttrPhaseOutcome+".truncated", true))
		}
	}
	hypSpan.End()

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

	enrichCtx, enrichSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.enrichment",
		trace.WithAttributes(
			attribute.String(telemetry.AttrPhase, "enrichment"),
			attribute.Int(telemetry.AttrPhaseIndex, 3),
		))

	if !skipLLM {
		log.Printf("phase 3: starting enrichment agent loop")

		toolServer, toolErr := tools.New(ctx, llmClient)
		if toolErr != nil {
			enrichSpan.RecordError(toolErr)
			enrichSpan.SetStatus(codes.Error, toolErr.Error())
			enrichSpan.End()
			return fail("start tool server: %w", toolErr)
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

		enrichTrace, enrichSummary, enrichErr := agent.Run(enrichCtx, inv, llmClient, toolServer.Client, toolCB)
		if enrichErr != nil {
			enrichSpan.RecordError(enrichErr)
			enrichSpan.SetStatus(codes.Error, enrichErr.Error())
			enrichSpan.End()
			return fail("enrichment: %w", enrichErr)
		}
		log.Printf("phase 3: complete — %d tool calls", len(enrichTrace))

		traceJSON, marshalErr := json.Marshal(enrichTrace)
		if marshalErr != nil {
			enrichSpan.RecordError(marshalErr)
			enrichSpan.SetStatus(codes.Error, marshalErr.Error())
			enrichSpan.End()
			return fail("marshal enrichment trace: %w", marshalErr)
		}
		if storeErr := db.UpdateEnrichment(ctx, conn, invID, traceJSON, enrichSummary); storeErr != nil {
			enrichSpan.RecordError(storeErr)
			enrichSpan.SetStatus(codes.Error, storeErr.Error())
			enrichSpan.End()
			return fail("store enrichment: %w", storeErr)
		}
	}
	enrichSpan.End()

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

	synthCtx, synthSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "ssspy.phase.synthesis",
		trace.WithAttributes(
			attribute.String(telemetry.AttrPhase, "synthesis"),
			attribute.Int(telemetry.AttrPhaseIndex, 4),
		))

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
		var synthErr error
		synthResult, synthErr = synthesis.Generate(synthCtx, llmClient, inv)
		if synthErr != nil {
			synthSpan.RecordError(synthErr)
			synthSpan.SetStatus(codes.Error, synthErr.Error())
			synthSpan.End()
			return fail("synthesis: %w", synthErr)
		}
	}

	synthJSON, err := json.Marshal(synthResult)
	if err != nil {
		synthSpan.RecordError(err)
		synthSpan.SetStatus(codes.Error, err.Error())
		synthSpan.End()
		return fail("marshal synthesis result: %w", err)
	}
	if v, trunc := telemetry.Truncate(string(synthJSON)); true {
		synthSpan.SetAttributes(attribute.String(telemetry.AttrPhaseOutcome, v))
		if trunc {
			synthSpan.SetAttributes(attribute.Bool(telemetry.AttrPhaseOutcome+".truncated", true))
		}
	}
	synthSpan.End()

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
