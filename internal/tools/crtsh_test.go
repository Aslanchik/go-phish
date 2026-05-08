package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// buildCrtshPayload generates a JSON array of n crt.sh-style entries.
func buildCrtshPayload(n int) []byte {
	entries := make([]map[string]string, n)
	for i := 0; i < n; i++ {
		entries[i] = map[string]string{
			"common_name": fmt.Sprintf("sub%d.example.com", i),
			"name_value":  fmt.Sprintf("sub%d.example.com\n*.sub%d.example.com", i, i),
			"issuer_name": "C=US, O=Let's Encrypt, CN=R3",
			"not_before":  "2024-01-01T00:00:00",
			"not_after":   "2024-04-01T00:00:00",
		}
	}
	b, _ := json.Marshal(entries)
	return b
}

// callHandler sends a cert_transparency tool request using the provided domain
// and an overridden HTTP client base URL pointing at srv.
func callHandler(t *testing.T, srv *httptest.Server, domain string) certTransparencyResult {
	t.Helper()

	// Swap the real crt.sh URL with the test server URL by monkey-patching via
	// a closure that wraps certTransparencyHandler with a custom HTTP client and URL.
	// Because the handler constructs its own http.Client internally we inject the
	// test server via an exported variable (crtshBaseURL) declared for testing.
	origURL := crtshBaseURL
	crtshBaseURL = srv.URL + "/"
	defer func() { crtshBaseURL = origURL }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"domain": domain,
	}

	result, err := certTransparencyHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	// Extract the text content from the result.
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	if text == "" {
		t.Fatalf("no text content in result: %+v", result)
	}

	var out certTransparencyResult
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal tool result %q: %v", text, err)
	}
	return out
}

// TestCertTransparency_FieldMappingAndCap verifies that the handler correctly
// maps crt.sh JSON fields to the output schema and caps results at 50 entries.
func TestCertTransparency_FieldMappingAndCap(t *testing.T) {
	const totalEntries = 51

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildCrtshPayload(totalEntries))
	}))
	defer srv.Close()

	out := callHandler(t, srv, "example.com")

	if out.Error != "" {
		t.Fatalf("unexpected error in result: %s", out.Error)
	}
	if len(out.Certificates) != crtshMaxResults {
		t.Errorf("expected %d certificates (cap), got %d", crtshMaxResults, len(out.Certificates))
	}

	first := out.Certificates[0]
	if first.CommonName != "sub0.example.com" {
		t.Errorf("unexpected common_name %q", first.CommonName)
	}
	if len(first.SANEntries) != 2 {
		t.Errorf("expected 2 SAN entries, got %d: %v", len(first.SANEntries), first.SANEntries)
	}
	if !strings.Contains(first.Issuer, "Let's Encrypt") {
		t.Errorf("unexpected issuer %q", first.Issuer)
	}
	// not_before should be converted to RFC3339
	if first.NotBefore != "2024-01-01T00:00:00Z" {
		t.Errorf("unexpected not_before %q (want RFC3339)", first.NotBefore)
	}
	if first.NotAfter != "2024-04-01T00:00:00Z" {
		t.Errorf("unexpected not_after %q (want RFC3339)", first.NotAfter)
	}
}

// TestCertTransparency_HTTPError verifies that a non-200 response from crt.sh
// is returned as a structured tool error (not a Go error / panic).
func TestCertTransparency_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	out := callHandler(t, srv, "example.com")

	if out.Error == "" {
		t.Fatal("expected non-empty error field for 500 response")
	}
	if out.Certificates == nil {
		t.Error("certificates field must not be nil on error")
	}
	if len(out.Certificates) != 0 {
		t.Errorf("expected empty certificates on error, got %d", len(out.Certificates))
	}
}

// TestCertTransparency_EmptyDomain verifies that a missing domain argument
// returns a structured error without panicking.
func TestCertTransparency_EmptyDomain(t *testing.T) {
	// No test server needed — handler should short-circuit before making any request.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"domain": "",
	}

	result, err := certTransparencyHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}

	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}

	var out certTransparencyResult
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if out.Error == "" {
		t.Error("expected error for empty domain")
	}
}
