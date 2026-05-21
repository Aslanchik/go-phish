package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/telemetry"
)

const (
	model           = anthropic.ModelClaudeSonnet4_6
	maxTokens       = 4096
	defaultMaxTurns = 10
)

// ToolCall records a single tool invocation and its result.
type ToolCall struct {
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Output   json.RawMessage `json:"output"`
	CalledAt time.Time       `json:"called_at"`
}

// Run executes the Phase 3 enrichment agent loop.
// It calls Claude with the available MCP tools, dispatches tool calls back through
// the MCP client, and repeats until Claude responds with no tool calls or the
// iteration cap is reached. Tool errors are returned to the model as structured
// results — they do not abort the loop.
//
// toolCB is called twice per dispatch: once before (output == nil) and once after
// (output set). Pass nil to disable.
func Run(
	ctx context.Context,
	inv db.Investigation,
	anthropicClient *anthropic.Client,
	mcpClient *mcpclient.Client,
	toolCB func(toolName string, input, output json.RawMessage),
) (trace []ToolCall, summary string, err error) {
	tools, err := buildToolList(ctx, mcpClient)
	if err != nil {
		return nil, "", fmt.Errorf("list MCP tools: %w", err)
	}

	maxTurns := defaultMaxTurns
	if v := os.Getenv("ENRICHMENT_MAX_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTurns = n
		}
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(buildInitialPrompt(inv))),
	}

	for turn := 0; turn < maxTurns; turn++ {
		log.Printf("enrichment: turn %d/%d", turn+1, maxTurns)

		spanName := "chat " + string(model)
		llmCtx, llmSpan := otel.Tracer(telemetry.TracerName).Start(ctx, spanName,
			oteltrace.WithAttributes(
				attribute.String(telemetry.AttrGenAIOperationName, "chat"),
				attribute.String(telemetry.AttrGenAIProviderName, "anthropic"),
				attribute.String(telemetry.AttrGenAIRequestModel, string(model)),
				attribute.Int(telemetry.AttrGenAIRequestMaxTokens, maxTokens),
			))

		resp, err := anthropicClient.Messages.New(llmCtx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: maxTokens,
			Messages:  messages,
			Tools:     tools,
		})
		if err != nil {
			llmSpan.RecordError(err)
			llmSpan.SetStatus(codes.Error, err.Error())
			llmSpan.End()
			return trace, summary, fmt.Errorf("anthropic API (turn %d): %w", turn+1, err)
		}
		setLLMResponseAttrs(llmSpan, resp)
		llmSpan.End()

		// Append assistant turn to history.
		messages = append(messages, resp.ToParam())

		// Collect tool calls from this response.
		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			tu := block.AsToolUse()
			if tu.Name == "" {
				continue
			}

			log.Printf("enrichment: → %s %s", tu.Name, truncate(string(tu.Input), 120))
			if toolCB != nil {
				toolCB(tu.Name, json.RawMessage(tu.Input), nil)
			}

			toolCtx, toolSpan := otel.Tracer(telemetry.TracerName).Start(ctx, "execute_tool "+tu.Name,
				oteltrace.WithAttributes(
					attribute.String(telemetry.AttrGenAIToolName, tu.Name),
				))
			if tu.ID != "" {
				toolSpan.SetAttributes(attribute.String(telemetry.AttrGenAIToolCallID, tu.ID))
			}
			inputV, inputTrunc := telemetry.Truncate(string(tu.Input))
			toolSpan.SetAttributes(attribute.String(telemetry.AttrToolInput, inputV))
			if inputTrunc {
				toolSpan.SetAttributes(attribute.Bool(telemetry.AttrToolInput+".truncated", true))
			}

			output, callErr := dispatchTool(toolCtx, mcpClient, tu.Name, tu.Input)
			if toolCB != nil {
				toolCB(tu.Name, json.RawMessage(tu.Input), output)
			}

			tc := ToolCall{
				Tool:     tu.Name,
				Input:    json.RawMessage(tu.Input),
				Output:   output,
				CalledAt: time.Now().UTC(),
			}
			if callErr != nil {
				errJSON, _ := json.Marshal(map[string]string{"error": callErr.Error()})
				tc.Output = errJSON
				toolSpan.RecordError(callErr)
				toolSpan.SetStatus(codes.Error, callErr.Error())
			} else {
				outputV, outputTrunc := telemetry.Truncate(string(output))
				toolSpan.SetAttributes(attribute.String(telemetry.AttrToolOutput, outputV))
				if outputTrunc {
					toolSpan.SetAttributes(attribute.Bool(telemetry.AttrToolOutput+".truncated", true))
				}
			}
			toolSpan.End()

			trace = append(trace, tc)
			toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, string(tc.Output), false))
		}

		// No tool calls — the model is done.
		if len(toolResults) == 0 {
			for _, block := range resp.Content {
				if t := block.AsText(); t.Text != "" {
					summary = t.Text
					break
				}
			}
			log.Printf("enrichment: model signalled done — %d tool calls across %d turns", len(trace), turn+1)
			return trace, summary, nil
		}

		// Feed tool results back as a new user message.
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	// Cap reached — return what we have.
	log.Printf("enrichment: iteration cap reached (%d turns) — %d tool calls", maxTurns, len(trace))
	return trace, summary, nil
}

func setLLMResponseAttrs(span oteltrace.Span, resp *anthropic.Message) {
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

// truncate shortens s to at most n bytes, appending "…" if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// buildToolList fetches tools from the MCP server and converts them to the
// Anthropic tool definition format.
func buildToolList(ctx context.Context, c *mcpclient.Client) ([]anthropic.ToolUnionParam, error) {
	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	tools := make([]anthropic.ToolUnionParam, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, mcpToolToAnthropic(t))
	}
	return tools, nil
}

// mcpToolToAnthropic converts an MCP tool definition to an Anthropic tool param.
func mcpToolToAnthropic(t mcp.Tool) anthropic.ToolUnionParam {
	schemaBytes, _ := json.Marshal(t.InputSchema)
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	_ = json.Unmarshal(schemaBytes, &schema)

	tool := anthropic.ToolParam{
		Name:        t.Name,
		Description: param.NewOpt(t.Description),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: schema.Properties,
		},
	}
	return anthropic.ToolUnionParam{OfTool: &tool}
}

// dispatchTool calls a single tool on the MCP server and returns its JSON output.
func dispatchTool(ctx context.Context, c *mcpclient.Client, name string, input json.RawMessage) (json.RawMessage, error) {
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: input,
		},
	})
	if err != nil {
		return nil, err
	}

	// Extract text content from the result.
	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			return json.RawMessage(tc.Text), nil
		}
	}
	return json.RawMessage(`{}`), nil
}

// buildInitialPrompt constructs the opening user message for the enrichment loop.
func buildInitialPrompt(inv db.Investigation) string {
	var h struct {
		Brand          string `json:"brand"`
		TargetedAction string `json:"targeted_action"`
		Confidence     string `json:"confidence"`
		Reasoning      string `json:"reasoning"`
	}
	_ = json.Unmarshal(inv.Hypothesis, &h)

	var forms []struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(inv.Forms, &forms)

	var jsFiles []json.RawMessage
	_ = json.Unmarshal(inv.JSFiles, &jsFiles)

	formActions := make([]string, 0, len(forms))
	for _, f := range forms {
		if f.Action != "" {
			formActions = append(formActions, f.Action)
		}
	}

	prompt := fmt.Sprintf(`You are a phishing analyst conducting targeted enrichment on a suspicious page.

## Phase 2 Hypothesis
- Brand impersonated: %s
- Targeted action: %s
- Confidence: %s
- Reasoning: %s

## Phase 1 Artifacts
- Final URL: %s
- Form action URLs: %v
- JavaScript files loaded: %d

Use the available tools to investigate this page further. Focus on:
1. Domain registration age (whois_lookup) — freshly registered domains are high-signal
2. Certificate transparency (cert_transparency) — unusual cert patterns
3. Prior scans and verdicts (urlscan_lookup, urlhaus_check)
4. JavaScript analysis (analyze_js) — look for exfiltration URLs and kit fingerprints

When you have gathered sufficient evidence, respond with a plain prose summary of your findings. No markdown headings, bullets, or formatting — write in complete sentences as you would in an incident report.`,
		h.Brand, h.TargetedAction, h.Confidence, h.Reasoning,
		inv.FinalURL.String,
		formActions,
		len(jsFiles),
	)

	return prompt
}
