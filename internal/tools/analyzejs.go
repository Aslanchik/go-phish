package tools

import (
	"context"
	"encoding/json"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	analyzeJSMaxChars = 50_000
	analyzeJSModel    = anthropic.ModelClaudeSonnet4_6
	analyzeJSMaxTok   = 1024
)

type analyzeJSResult struct {
	KitName             string   `json:"kit_name"`
	ExfilURLs           []string `json:"exfil_urls"`
	ObfuscationDetected bool     `json:"obfuscation_detected"`
	NotableStrings      []string `json:"notable_strings"`
	Summary             string   `json:"summary"`
	Truncated           bool     `json:"truncated"`
	Error               string   `json:"error,omitempty"`
}

// analyzeJSModelOutput mirrors only the fields the model fills in via record_js_analysis.
type analyzeJSModelOutput struct {
	KitName             string   `json:"kit_name"`
	ExfilURLs           []string `json:"exfil_urls"`
	ObfuscationDetected bool     `json:"obfuscation_detected"`
	NotableStrings      []string `json:"notable_strings"`
	Summary             string   `json:"summary"`
}

func analyzeJSErrorResult(reason string) (*mcp.CallToolResult, error) {
	b, _ := json.Marshal(analyzeJSResult{
		ExfilURLs:      []string{},
		NotableStrings: []string{},
		Error:          reason,
	})
	return mcp.NewToolResultText(string(b)), nil
}

// truncateJS truncates js to analyzeJSMaxChars and reports whether truncation occurred.
func truncateJS(js string) (string, bool) {
	if len(js) <= analyzeJSMaxChars {
		return js, false
	}
	return js[:analyzeJSMaxChars], true
}

func recordJSAnalysisTool() anthropic.ToolUnionParam {
	t := anthropic.ToolParam{
		Name: "record_js_analysis",
		Description: param.NewOpt("Record a structured analysis of the JavaScript content from a phishing page."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"kit_name": map[string]any{
					"type":        "string",
					"description": "Name of the phishing kit if identifiable, empty string otherwise.",
				},
				"exfil_urls": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "URLs where credentials or data are sent.",
				},
				"obfuscation_detected": map[string]any{
					"type":        "boolean",
					"description": "True if the JS uses obfuscation techniques (eval, hex encoding, etc.).",
				},
				"notable_strings": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Strings of interest: email addresses, domains, hardcoded tokens.",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "Two to three sentence summary of what the script does.",
				},
			},
			Required: []string{"kit_name", "exfil_urls", "obfuscation_detected", "notable_strings", "summary"},
		},
	}
	return anthropic.ToolUnionParam{OfTool: &t}
}

// makeAnalyzeJSHandler returns a handler closure that holds the Anthropic client.
func makeAnalyzeJSHandler(client *anthropic.Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if os.Getenv("DISABLE_ANALYZE_JS") == "1" {
			return analyzeJSErrorResult("analyze_js is disabled (DISABLE_ANALYZE_JS=1)")
		}

		jsContent, err := req.RequireString("js_content")
		if err != nil {
			return analyzeJSErrorResult("missing or empty 'js_content' argument")
		}

		truncated := false
		jsContent, truncated = truncateJS(jsContent)

		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     analyzeJSModel,
			MaxTokens: analyzeJSMaxTok,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(
					anthropic.NewTextBlock("Analyse this JavaScript from a suspected phishing page:\n\n" + jsContent),
				),
			},
			Tools:      []anthropic.ToolUnionParam{recordJSAnalysisTool()},
			ToolChoice: anthropic.ToolChoiceParamOfTool("record_js_analysis"),
		})
		if err != nil {
			return analyzeJSErrorResult("anthropic API error: " + err.Error())
		}

		for _, block := range resp.Content {
			tu := block.AsToolUse()
			if tu.Name != "record_js_analysis" {
				continue
			}
			var modelOut analyzeJSModelOutput
			if err := json.Unmarshal(tu.Input, &modelOut); err != nil {
				return analyzeJSErrorResult("failed to parse model output: " + err.Error())
			}

			exfilURLs := modelOut.ExfilURLs
			if exfilURLs == nil {
				exfilURLs = []string{}
			}
			notable := modelOut.NotableStrings
			if notable == nil {
				notable = []string{}
			}

			out := analyzeJSResult{
				KitName:             modelOut.KitName,
				ExfilURLs:           exfilURLs,
				ObfuscationDetected: modelOut.ObfuscationDetected,
				NotableStrings:      notable,
				Summary:             modelOut.Summary,
				Truncated:           truncated,
			}
			b, err := json.Marshal(out)
			if err != nil {
				return analyzeJSErrorResult("failed to serialise result: " + err.Error())
			}
			return mcp.NewToolResultText(string(b)), nil
		}

		return analyzeJSErrorResult("model did not call record_js_analysis")
	}
}
