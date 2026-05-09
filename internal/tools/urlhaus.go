package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	urlhausMaxURLs  = 20
	urlhausTimeout  = 15 * time.Second
)

// urlhausBaseURLEndpoint and urlhausBaseHostEndpoint are overridable in tests.
var (
	urlhausURLEndpoint  = "https://urlhaus-api.abuse.ch/v1/url/"
	urlhausHostEndpoint = "https://urlhaus-api.abuse.ch/v1/host/"
)

type urlhausResult struct {
	Found       bool     `json:"found"`
	ThreatType  string   `json:"threat_type"`
	Tags        []string `json:"tags"`
	DateAdded   string   `json:"date_added"`
	URLsOnHost  []string `json:"urls_on_host"`
	Error       string   `json:"error,omitempty"`
}

// urlhausURLResponse mirrors the URLhaus /url/ API response.
type urlhausURLResponse struct {
	QueryStatus string   `json:"query_status"`
	ThreatType  string   `json:"threat_type"`
	Tags        []string `json:"tags"`
	DateAdded   string   `json:"date_added"`
}

// urlhausHostResponse mirrors the URLhaus /host/ API response.
type urlhausHostResponse struct {
	QueryStatus string `json:"query_status"`
	URLs        []struct {
		URL string `json:"url"`
	} `json:"urls"`
}

func urlhausErrorResult(reason string) (*mcp.CallToolResult, error) {
	b, _ := json.Marshal(urlhausResult{Tags: []string{}, URLsOnHost: []string{}, Error: reason})
	return mcp.NewToolResultText(string(b)), nil
}

func urlhausHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := req.RequireString("url_or_domain")
	if err != nil || strings.TrimSpace(input) == "" {
		return urlhausErrorResult("missing or empty 'url_or_domain' argument")
	}
	input = strings.TrimSpace(input)

	client := &http.Client{Timeout: urlhausTimeout}

	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return urlhausLookupURL(client, input)
	}
	return urlhausLookupHost(client, input)
}

func urlhausLookupURL(client *http.Client, rawURL string) (*mcp.CallToolResult, error) {
	resp, err := client.PostForm(urlhausURLEndpoint, url.Values{"url": {rawURL}})
	if err != nil {
		return urlhausErrorResult("urlhaus url request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return urlhausErrorResult(fmt.Sprintf("urlhaus returned status %d", resp.StatusCode))
	}

	var apiResp urlhausURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return urlhausErrorResult("failed to parse urlhaus response: " + err.Error())
	}

	if apiResp.QueryStatus == "no_results" {
		out := urlhausResult{Found: false, Tags: []string{}, URLsOnHost: []string{}}
		b, _ := json.Marshal(out)
		return mcp.NewToolResultText(string(b)), nil
	}

	tags := apiResp.Tags
	if tags == nil {
		tags = []string{}
	}

	out := urlhausResult{
		Found:      true,
		ThreatType: apiResp.ThreatType,
		Tags:       tags,
		DateAdded:  normaliseURLhausTime(apiResp.DateAdded),
		URLsOnHost: []string{},
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

func urlhausLookupHost(client *http.Client, host string) (*mcp.CallToolResult, error) {
	resp, err := client.PostForm(urlhausHostEndpoint, url.Values{"host": {host}})
	if err != nil {
		return urlhausErrorResult("urlhaus host request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return urlhausErrorResult(fmt.Sprintf("urlhaus returned status %d", resp.StatusCode))
	}

	var apiResp urlhausHostResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return urlhausErrorResult("failed to parse urlhaus response: " + err.Error())
	}

	if apiResp.QueryStatus == "no_results" {
		out := urlhausResult{Found: false, Tags: []string{}, URLsOnHost: []string{}}
		b, _ := json.Marshal(out)
		return mcp.NewToolResultText(string(b)), nil
	}

	urlsOnHost := make([]string, 0, len(apiResp.URLs))
	for _, u := range apiResp.URLs {
		urlsOnHost = append(urlsOnHost, u.URL)
	}
	if len(urlsOnHost) > urlhausMaxURLs {
		urlsOnHost = urlsOnHost[:urlhausMaxURLs]
	}

	out := urlhausResult{
		Found:      true,
		Tags:       []string{},
		URLsOnHost: urlsOnHost,
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// normaliseURLhausTime converts URLhaus datetime strings to RFC3339.
// URLhaus returns dates like "2023-01-02 15:04:05 UTC".
func normaliseURLhausTime(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), " UTC")
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}
