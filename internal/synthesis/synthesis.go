package synthesis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/aslanchik/go-phish/internal/db"
)

// ErrNoToolCall is returned when the model responds without calling record_synthesis.
var ErrNoToolCall = errors.New("model did not call record_synthesis")

// ErrNoHypothesis is returned when synthesis is attempted without a prior hypothesis.
var ErrNoHypothesis = errors.New("synthesis requires a prior hypothesis; inv.Hypothesis is null")

// Claim is a single finding with an evidence-backed confidence rating.
type Claim struct {
	Value      string `json:"value"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
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

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_6,
		MaxTokens: 2048,
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
		return Result{}, fmt.Errorf("anthropic API: %w", err)
	}

	return parseBlocks(resp.Content)
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
	claim := map[string]any{
		"type":     "object",
		"required": []string{"value", "confidence", "evidence"},
		"properties": map[string]any{
			"value":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"evidence":   map[string]any{"type": "string", "minLength": 1},
		},
	}
	verdictClaim := map[string]any{
		"type":     "object",
		"required": []string{"value", "confidence", "evidence"},
		"properties": map[string]any{
			"value":      map[string]any{"type": "string", "enum": []string{"phishing", "benign", "inconclusive"}},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"evidence":   map[string]any{"type": "string", "minLength": 1},
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

You MUST call the record_synthesis tool with your findings. Do not write a text response.`
