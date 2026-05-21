package hypothesis

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aslanchik/go-phish/internal/telemetry"
)

const (
	model     = anthropic.ModelClaudeSonnet4_6
	maxTokens = 1024
)

// ErrNoToolCall is returned when the model responds without calling record_hypothesis.
var ErrNoToolCall = errors.New("model did not call record_hypothesis; cannot parse hypothesis")

// Generate calls the Anthropic API with a screenshot and rendered DOM to produce
// a structured phishing hypothesis. The client must be initialised with a valid
// ANTHROPIC_API_KEY before calling this function.
func Generate(ctx context.Context, client *anthropic.Client, screenshotPNG []byte, dom string) (Hypothesis, error) {
	summary, err := DOMSummary(dom, nil)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("build DOM summary: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(screenshotPNG)

	spanName := "chat " + string(model)
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String(telemetry.AttrGenAIOperationName, "chat"),
			attribute.String(telemetry.AttrGenAIProviderName, "anthropic"),
			attribute.String(telemetry.AttrGenAIRequestModel, string(model)),
			attribute.Int(telemetry.AttrGenAIRequestMaxTokens, maxTokens),
			attribute.String(telemetry.AttrScreenshotContentType, "image/png"),
			attribute.Int64(telemetry.AttrScreenshotSizeBytes, int64(len(screenshotPNG))),
			attribute.String(telemetry.AttrScreenshotSHA256, screenshotSHA256(screenshotPNG)),
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Hypothesis{}, fmt.Errorf("anthropic API: %w", err)
	}

	setLLMResponseAttrs(span, resp)

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
	return Hypothesis{}, ErrNoToolCall
}

func screenshotSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
