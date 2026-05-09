package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

const urlhausFoundURLResponse = `{
	"query_status": "is_whitelisted",
	"threat_type": "phishing",
	"tags": ["phishing", "credential-stealer"],
	"date_added": "2024-03-15 10:00:00 UTC"
}`

const urlhausFoundHostResponse = `{
	"query_status": "is_host",
	"urls": [
		{"url": "https://evil.com/login"},
		{"url": "https://evil.com/steal"},
		{"url": "https://evil.com/exfil"}
	]
}`

const urlhausNoResults = `{"query_status": "no_results"}`

func urlhausExtractResult(t *testing.T, result *mcp.CallToolResult) urlhausResult {
	t.Helper()
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var out urlhausResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, tc.Text)
	}
	return out
}

func TestURLhaus_FoundURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, urlhausFoundURLResponse)
	}))
	defer srv.Close()

	urlhausURLEndpoint = srv.URL + "/"
	defer func() { urlhausURLEndpoint = "https://urlhaus-api.abuse.ch/v1/url/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url_or_domain": "https://evil.com/login"}

	result, err := urlhausHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	out := urlhausExtractResult(t, result)

	if !out.Found {
		t.Error("expected found=true")
	}
	if out.ThreatType != "phishing" {
		t.Errorf("threat_type: got %q want %q", out.ThreatType, "phishing")
	}
	if len(out.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(out.Tags))
	}
	if out.DateAdded == "" {
		t.Error("date_added should be populated")
	}
}

func TestURLhaus_FoundHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, urlhausFoundHostResponse)
	}))
	defer srv.Close()

	urlhausHostEndpoint = srv.URL + "/"
	defer func() { urlhausHostEndpoint = "https://urlhaus-api.abuse.ch/v1/host/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url_or_domain": "evil.com"}

	result, err := urlhausHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	out := urlhausExtractResult(t, result)

	if !out.Found {
		t.Error("expected found=true")
	}
	if len(out.URLsOnHost) != 3 {
		t.Errorf("expected 3 urls_on_host, got %d", len(out.URLsOnHost))
	}
}

func TestURLhaus_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, urlhausNoResults)
	}))
	defer srv.Close()

	urlhausURLEndpoint = srv.URL + "/"
	defer func() { urlhausURLEndpoint = "https://urlhaus-api.abuse.ch/v1/url/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url_or_domain": "https://clean.com/page"}

	result, err := urlhausHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	out := urlhausExtractResult(t, result)

	if out.Found {
		t.Error("expected found=false for no_results")
	}
	if out.Error != "" {
		t.Errorf("no_results should not set error field, got: %s", out.Error)
	}
}

func TestURLhaus_URLsOnHostCap(t *testing.T) {
	urls := make([]map[string]string, 25)
	for i := range urls {
		urls[i] = map[string]string{"url": fmt.Sprintf("https://evil.com/path%d", i)}
	}
	b, _ := json.Marshal(map[string]any{
		"query_status": "is_host",
		"urls":         urls,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	urlhausHostEndpoint = srv.URL + "/"
	defer func() { urlhausHostEndpoint = "https://urlhaus-api.abuse.ch/v1/host/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url_or_domain": "evil.com"}

	result, err := urlhausHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	out := urlhausExtractResult(t, result)

	if len(out.URLsOnHost) != urlhausMaxURLs {
		t.Errorf("expected cap of %d urls_on_host, got %d", urlhausMaxURLs, len(out.URLsOnHost))
	}
}
