package hypothesis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const (
	model     = anthropic.ModelClaudeSonnet4_6
	maxTokens = 1024
)

// ErrNoToolCall is returned when the model responds without calling record_hypothesis.
type ErrNoToolCall struct{}

func (ErrNoToolCall) Error() string {
	return "model did not call record_hypothesis; cannot parse hypothesis"
}

// Generate calls the Anthropic API with a screenshot and rendered DOM to produce
// a structured phishing hypothesis. The client must be initialised with a valid
// ANTHROPIC_API_KEY before calling this function.
func Generate(ctx context.Context, client *anthropic.Client, screenshotPNG []byte, dom string) (Hypothesis, error) {
	summary, err := DOMSummary(dom, nil)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("build DOM summary: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(screenshotPNG)

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64("image/png", b64),
				anthropic.NewTextBlock(summary.String()),
			),
		},
		Tools: []anthropic.ToolUnionParam{
			recordHypothesisTool(),
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool("record_hypothesis"),
	})
	if err != nil {
		return Hypothesis{}, fmt.Errorf("anthropic API: %w", err)
	}

	for _, block := range resp.Content {
		tu := block.AsToolUse()
		if tu.Name != "record_hypothesis" {
			continue
		}
		var h Hypothesis
		if err := json.Unmarshal(tu.Input, &h); err != nil {
			return Hypothesis{}, fmt.Errorf("unmarshal hypothesis: %w", err)
		}
		return h, nil
	}
	return Hypothesis{}, ErrNoToolCall{}
}

func recordHypothesisSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"brand": map[string]any{
				"type":        "string",
				"description": "The brand or organisation being impersonated (e.g. PayPal, Chase, Microsoft).",
			},
			"targeted_action": map[string]any{
				"type":        "string",
				"enum":        []string{"credential_theft", "payment_capture", "mfa_bypass", "other"},
				"description": "The primary action the kit is trying to elicit from the victim.",
			},
			"confidence": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "How confident you are that this is a phishing page targeting the identified brand.",
			},
			"reasoning": map[string]any{
				"type":        "string",
				"description": "One or two sentences explaining what visual or structural cues drove the confidence rating.",
			},
		},
		Required: []string{"brand", "targeted_action", "confidence", "reasoning"},
	}
}

// recordHypothesisTool returns the full ToolParam, adding a description alongside the schema.
// We add description via the ToolParam directly since ToolUnionParamOfTool does not accept it.
func recordHypothesisTool() anthropic.ToolUnionParam {
	t := anthropic.ToolParam{
		Name:        "record_hypothesis",
		InputSchema: recordHypothesisSchema(),
		Description: param.NewOpt("Record a structured phishing hypothesis about the page under investigation."),
	}
	return anthropic.ToolUnionParam{OfTool: &t}
}

const systemPrompt = `You are a phishing analyst. You will be shown a screenshot and a structured
summary of a suspicious web page. Your task is to identify whether it is a phishing page, which
brand it impersonates, what victim action it targets, and how confident you are.

You MUST call the record_hypothesis tool with your findings. Do not write a text response.`
