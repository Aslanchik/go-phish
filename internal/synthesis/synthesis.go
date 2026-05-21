package synthesis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/telemetry"
)

const (
	model     = anthropic.ModelClaudeSonnet4_6
	maxTokens = 2048
)

// ErrNoToolCall is returned when the model responds without calling record_synthesis.
var ErrNoToolCall = errors.New("model did not call record_synthesis")

// ErrNoHypothesis is returned when synthesis is attempted without a prior hypothesis.
var ErrNoHypothesis = errors.New("synthesis requires a prior hypothesis; inv.Hypothesis is null")

// Claim is a single finding with an evidence-backed confidence rating.
type Claim struct {
	Value      string   `json:"value"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

// Result holds the five structured findings produced by Phase 4.
type Result struct {
	BrandImpersonated   Claim `json:"brand_impersonated"`
	KitIdentification   Claim `json:"kit_identification"`
	ExfilTarget         Claim `json:"exfil_target"`
	InfrastructureNotes Claim `json:"infrastructure_notes"`
	Verdict             Claim `json:"verdict"`
}

// Generate calls the Anthropic API to synthesise the findings from Phases 1–3 into
// a structured verdict. It forces the model to call record_synthesis via tool_use.
func Generate(ctx context.Context, client *anthropic.Client, inv db.Investigation) (Result, error) {
	if len(inv.Hypothesis) == 0 {
		return Result{}, ErrNoHypothesis
	}

	spanName := "chat " + string(model)
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String(telemetry.AttrGenAIOperationName, "chat"),
			attribute.String(telemetry.AttrGenAIProviderName, "anthropic"),
			attribute.String(telemetry.AttrGenAIRequestModel, string(model)),
			attribute.Int(telemetry.AttrGenAIRequestMaxTokens, maxTokens),
		))
	defer span.End()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(hypothesisBlock(inv.Hypothesis)),
				anthropic.NewTextBlock(enrichmentBlock(inv.EnrichmentTrace)),
				anthropic.NewTextBlock(artifactBlock(inv.FinalURL, inv.Forms, inv.JSFiles)),
			),
		},
		Tools: []anthropic.ToolUnionParam{
			recordSynthesisTool(),
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool("record_synthesis"),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Result{}, fmt.Errorf("anthropic API: %w", err)
	}

	setLLMResponseAttrs(span, resp)
	return parseBlocks(resp.Content)
}

func setLLMResponseAttrs(span trace.Span, resp *anthropic.Message) {
	span.SetAttributes(
		attribute.String(telemetry.AttrGenAIResponseModel, string(resp.Model)),
		attribute.String(telemetry.AttrGenAIResponseID, resp.ID),
		attribute.Int64(telemetry.AttrGenAIUsageInputTokens,
			resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens),
		attribute.Int64(telemetry.AttrGenAIUsageOutputTokens, resp.Usage.OutputTokens),
		attribute.Int64(telemetry.AttrGenAIUsageCacheCreationInputTokens, resp.Usage.CacheCreationInputTokens),
		attribute.Int64(telemetry.AttrGenAIUsageCacheReadInputTokens, resp.Usage.CacheReadInputTokens),
	)
	if resp.StopReason != "" {
		span.SetAttributes(attribute.StringSlice(telemetry.AttrGenAIResponseFinishReasons, []string{string(resp.StopReason)}))
	}
}

func parseBlocks(blocks []anthropic.ContentBlockUnion) (Result, error) {
	for _, block := range blocks {
		tu := block.AsToolUse()
		if tu.Name != "record_synthesis" {
			continue
		}
		var r Result
		if err := json.Unmarshal(tu.Input, &r); err != nil {
			return Result{}, fmt.Errorf("unmarshal synthesis result: %w", err)
		}
		return r, nil
	}
	return Result{}, ErrNoToolCall
}

func hypothesisBlock(raw json.RawMessage) string {
	return fmt.Sprintf("## Phase 2 Hypothesis\n%s", string(raw))
}

func enrichmentBlock(raw json.RawMessage) string {
	trace := "[]"
	if len(raw) > 0 {
		trace = string(raw)
	}
	return fmt.Sprintf("## Phase 3 Enrichment Evidence\n%s", trace)
}

func artifactBlock(finalURL sql.NullString, formsRaw, jsFilesRaw json.RawMessage) string {
	var forms []struct {
		Action string `json:"action"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(formsRaw, &forms)

	var jsFiles []json.RawMessage
	_ = json.Unmarshal(jsFilesRaw, &jsFiles)

	actions := make([]string, 0, len(forms))
	for _, f := range forms {
		if f.Action != "" {
			actions = append(actions, fmt.Sprintf("%s %s", f.Method, f.Action))
		}
	}

	return fmt.Sprintf("## Page Artifacts\nFinal URL: %s\nForm actions: %v\nJS files loaded: %d",
		finalURL.String, actions, len(jsFiles))
}

func recordSynthesisSchema() anthropic.ToolInputSchemaParam {
	evidenceSchema := map[string]any{
		"type":     "array",
		"items":    map[string]any{"type": "string"},
		"minItems": 1,
	}
	claim := map[string]any{
		"type":     "object",
		"required": []string{"value", "confidence", "evidence"},
		"properties": map[string]any{
			"value":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"evidence":   evidenceSchema,
		},
	}
	verdictClaim := map[string]any{
		"type":     "object",
		"required": []string{"value", "confidence", "evidence"},
		"properties": map[string]any{
			"value":      map[string]any{"type": "string", "enum": []string{"phishing", "benign", "inconclusive"}},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"evidence":   evidenceSchema,
		},
	}
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"brand_impersonated":   claim,
			"kit_identification":   claim,
			"exfil_target":         claim,
			"infrastructure_notes": claim,
			"verdict":              verdictClaim,
		},
		Required: []string{"brand_impersonated", "kit_identification", "exfil_target", "infrastructure_notes", "verdict"},
	}
}

func recordSynthesisTool() anthropic.ToolUnionParam {
	t := anthropic.ToolParam{
		Name:        "record_synthesis",
		InputSchema: recordSynthesisSchema(),
		Description: param.NewOpt("Record the structured synthesis findings for the phishing investigation."),
	}
	return anthropic.ToolUnionParam{OfTool: &t}
}

const systemPrompt = `You are a phishing analyst completing a final synthesis of the evidence collected during an investigation.

You will receive three blocks of evidence: the Phase 2 hypothesis (initial visual assessment), the Phase 3 enrichment trace (tool call results), and a summary of page artifacts.

Your task is to synthesise all evidence into a structured verdict. Every claim MUST be backed by specific evidence — cite the tool or artifact that supports each claim. Do not invent evidence.

The "evidence" field for each claim must be an array of short, discrete bullet points (one finding per item). Each item should cite the specific tool or artifact. Example: ["whois_lookup: domain registered 2026-05-01", "urlhaus_check: no matches found"].

You MUST call the record_synthesis tool with your findings. Do not write a text response.`
