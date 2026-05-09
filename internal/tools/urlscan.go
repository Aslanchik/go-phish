package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	urlscanMaxResults = 10
	urlscanTimeout    = 15 * time.Second
)

// urlscanBaseURL is overridable in tests.
var urlscanBaseURL = "https://urlscan.io/api/v1/search/"

type urlscanScan struct {
	ScanDate string   `json:"scan_date"`
	Verdict  string   `json:"verdict"`
	Tags     []string `json:"tags"`
	PageURL  string   `json:"page_url"`
}

type urlscanResult struct {
	Scans []urlscanScan `json:"scans"`
	Error string        `json:"error,omitempty"`
}

// urlscanAPIResponse mirrors the top-level shape of the urlscan.io search response.
type urlscanAPIResponse struct {
	Results []struct {
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
	} `json:"results"`
}

func urlscanErrorResult(reason string) (*mcp.CallToolResult, error) {
	b, _ := json.Marshal(urlscanResult{Scans: []urlscanScan{}, Error: reason})
	return mcp.NewToolResultText(string(b)), nil
}

func urlscanHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	apiKey := os.Getenv("URLSCAN_API_KEY")
	if apiKey == "" {
		return urlscanErrorResult("URLSCAN_API_KEY not set")
	}

	rawURL, err := req.RequireString("url")
	if err != nil || strings.TrimSpace(rawURL) == "" {
		return urlscanErrorResult("missing or empty 'url' argument")
	}
	rawURL = strings.TrimSpace(rawURL)

	reqURL := fmt.Sprintf("%s?q=page.url:%s", urlscanBaseURL, rawURL)
	httpReq, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return urlscanErrorResult("failed to build request: " + err.Error())
	}
	httpReq.Header.Set("Authorization", "API-Key "+apiKey)

	client := &http.Client{Timeout: urlscanTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return urlscanErrorResult("urlscan request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return urlscanErrorResult(fmt.Sprintf("urlscan returned status %d", resp.StatusCode))
	}

	var apiResp urlscanAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return urlscanErrorResult("failed to parse urlscan response: " + err.Error())
	}

	results := apiResp.Results
	if len(results) > urlscanMaxResults {
		results = results[:urlscanMaxResults]
	}

	scans := make([]urlscanScan, 0, len(results))
	for _, r := range results {
		verdict := "unknown"
		switch {
		case r.Verdicts.Overall.Malicious:
			verdict = "malicious"
		case r.Verdicts.Overall.Score > 0:
			verdict = "suspicious"
		default:
			verdict = "benign"
		}

		tags := r.Verdicts.Overall.Tags
		if tags == nil {
			tags = []string{}
		}

		scanDate := r.Task.Time
		if t, err := time.Parse(time.RFC3339, scanDate); err == nil {
			scanDate = t.UTC().Format(time.RFC3339)
		}

		scans = append(scans, urlscanScan{
			ScanDate: scanDate,
			Verdict:  verdict,
			Tags:     tags,
			PageURL:  r.Task.URL,
		})
	}

	out := urlscanResult{Scans: scans}
	b, err := json.Marshal(out)
	if err != nil {
		return urlscanErrorResult("failed to serialise result: " + err.Error())
	}
	return mcp.NewToolResultText(string(b)), nil
}
