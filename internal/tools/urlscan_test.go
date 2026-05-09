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

func buildURLScanPayload(n int) string {
	type result struct {
		Task struct {
			Time string `json:"time"`
			URL  string `json:"url"`
		} `json:"task"`
		Verdicts struct {
			Overall struct {
				Malicious bool     `json:"malicious"`
				Score     int      `json:"score"`
				Tags      []string `json:"tags"`
			} `json:"overall"`
		} `json:"verdicts"`
	}
	type response struct {
		Results []result `json:"results"`
	}

	resp := response{}
	for i := 0; i < n; i++ {
		var r result
		r.Task.Time = "2024-01-01T00:00:00Z"
		r.Task.URL = fmt.Sprintf("https://example.com/page%d", i)
		r.Verdicts.Overall.Malicious = i%3 == 0
		r.Verdicts.Overall.Score = i % 5
		r.Verdicts.Overall.Tags = []string{"phishing"}
		resp.Results = append(resp.Results, r)
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func TestURLScan_FieldMappingAndCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, buildURLScanPayload(15))
	}))
	defer srv.Close()

	t.Setenv("URLSCAN_API_KEY", "test-key")
	urlscanBaseURL = srv.URL + "/"
	defer func() { urlscanBaseURL = "https://urlscan.io/api/v1/search/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url": "https://example.com"}

	result, err := urlscanHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var out urlscanResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, tc.Text)
	}

	if out.Error != "" {
		t.Errorf("unexpected error: %s", out.Error)
	}
	if len(out.Scans) != urlscanMaxResults {
		t.Errorf("expected %d scans (cap), got %d", urlscanMaxResults, len(out.Scans))
	}
	if out.Scans[0].PageURL == "" {
		t.Error("page_url should be populated")
	}
	if out.Scans[0].ScanDate == "" {
		t.Error("scan_date should be populated")
	}
}

func TestURLScan_MissingAPIKey(t *testing.T) {
	t.Setenv("URLSCAN_API_KEY", "")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url": "https://example.com"}

	result, err := urlscanHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var out urlscanResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, tc.Text)
	}
	if out.Error == "" {
		t.Error("expected error for missing API key")
	}
}

func TestURLScan_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	t.Setenv("URLSCAN_API_KEY", "test-key")
	urlscanBaseURL = srv.URL + "/"
	defer func() { urlscanBaseURL = "https://urlscan.io/api/v1/search/" }()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"url": "https://example.com"}

	result, err := urlscanHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var out urlscanResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, tc.Text)
	}
	if out.Error == "" {
		t.Error("expected error for HTTP 503")
	}
}
