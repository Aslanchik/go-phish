package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestAnalyzeJS_Truncation(t *testing.T) {
	input := strings.Repeat("a", analyzeJSMaxChars+1000)
	out, wasTruncated := truncateJS(input)

	if !wasTruncated {
		t.Error("expected truncation for input > 50,000 chars")
	}
	if len(out) != analyzeJSMaxChars {
		t.Errorf("truncated length: got %d want %d", len(out), analyzeJSMaxChars)
	}
}

func TestAnalyzeJS_NoTruncation(t *testing.T) {
	input := strings.Repeat("x", 100)
	out, wasTruncated := truncateJS(input)

	if wasTruncated {
		t.Error("expected no truncation for short input")
	}
	if out != input {
		t.Error("short input should be returned unchanged")
	}
}

func TestAnalyzeJS_DisabledEnvVar(t *testing.T) {
	t.Setenv("DISABLE_ANALYZE_JS", "1")

	// nil client — handler must not reach the API call.
	handler := makeAnalyzeJSHandler((*anthropic.Client)(nil))

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"js_content": "console.log('hi')"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var out analyzeJSResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, tc.Text)
	}
	if out.Error == "" {
		t.Error("expected non-empty error when DISABLE_ANALYZE_JS=1")
	}
	if !strings.Contains(out.Error, "DISABLE_ANALYZE_JS") {
		t.Errorf("error message should mention DISABLE_ANALYZE_JS, got: %s", out.Error)
	}
}
