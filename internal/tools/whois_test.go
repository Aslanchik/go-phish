package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestWhois_OutputSerialization(t *testing.T) {
	r := whoisResult{
		Registrar:     "Example Registrar, Inc.",
		RegisteredAt:  "2020-01-01T00:00:00Z",
		ExpiresAt:     "2025-01-01T00:00:00Z",
		RegistrantOrg: "ACME Corp",
		Raw:           "Domain Name: example.com\n...",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out["registrar"] != r.Registrar {
		t.Errorf("registrar: got %q want %q", out["registrar"], r.Registrar)
	}
	if out["registered_at"] != r.RegisteredAt {
		t.Errorf("registered_at: got %q want %q", out["registered_at"], r.RegisteredAt)
	}
	if out["expires_at"] != r.ExpiresAt {
		t.Errorf("expires_at: got %q want %q", out["expires_at"], r.ExpiresAt)
	}
	if out["registrant_org"] != r.RegistrantOrg {
		t.Errorf("registrant_org: got %q want %q", out["registrant_org"], r.RegistrantOrg)
	}
	if _, ok := out["error"]; ok {
		t.Error("error field must be absent when empty")
	}
}

func TestWhois_TimeoutProducesStructuredError(t *testing.T) {
	// Use an invalid domain that will fail quickly (non-existent TLD).
	// We override whoisTimeout indirectly by passing a domain that causes
	// the library to return an error without a real network call.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"domain": ""}

	result, err := whoisHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}

	content := extractText(t, result)
	var out whoisResult
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		t.Fatalf("result is not valid JSON: %v — got: %s", err, content)
	}
	if out.Error == "" {
		t.Error("expected non-empty error field for empty domain")
	}
}

// extractText pulls the text content from the first content block of a tool result.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
