package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// crtshEntry mirrors a single record returned by the crt.sh JSON API.
type crtshEntry struct {
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
	IssuerName string `json:"issuer_name"`
	NotBefore  string `json:"not_before"`
	NotAfter   string `json:"not_after"`
}

// certResult is one certificate in the tool output.
type certResult struct {
	CommonName string   `json:"common_name"`
	SANEntries []string `json:"san_entries"`
	Issuer     string   `json:"issuer"`
	NotBefore  string   `json:"not_before"`
	NotAfter   string   `json:"not_after"`
}

// certTransparencyResult is the full tool output.
type certTransparencyResult struct {
	Certificates []certResult `json:"certificates"`
	Error        string       `json:"error,omitempty"`
}

const (
	crtshMaxResults = 50
	crtshTimeout    = 15 * time.Second
)

// crtshBaseURL is the base URL for the crt.sh API. Overridable in tests.
var crtshBaseURL = "https://crt.sh/"

// crtshTimeFormats lists the datetime layouts that crt.sh may return.
var crtshTimeFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05.999999999",
	time.RFC3339,
}

// parseCrtshTime parses a crt.sh datetime string and returns RFC3339.
// If parsing fails the original string is returned unchanged.
func parseCrtshTime(s string) string {
	for _, layout := range crtshTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

// toolErrorResult serialises a structured error result for the tool caller.
func toolErrorResult(reason string) (*mcp.CallToolResult, error) {
	out := certTransparencyResult{
		Certificates: []certResult{},
		Error:        reason,
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// certTransparencyHandler is the MCP tool handler for cert_transparency.
func certTransparencyHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil || strings.TrimSpace(domain) == "" {
		return toolErrorResult("missing or empty 'domain' argument")
	}
	domain = strings.TrimSpace(domain)

	apiURL := fmt.Sprintf("%s?q=%s&output=json", crtshBaseURL, domain)

	client := &http.Client{Timeout: crtshTimeout}
	resp, err := client.Get(apiURL)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("crt.sh request failed: %s", err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolErrorResult(fmt.Sprintf("crt.sh returned status %d", resp.StatusCode))
	}

	var raw []crtshEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return toolErrorResult(fmt.Sprintf("failed to parse crt.sh response: %s", err.Error()))
	}

	if len(raw) > crtshMaxResults {
		raw = raw[:crtshMaxResults]
	}

	certs := make([]certResult, 0, len(raw))
	for _, entry := range raw {
		sans := []string{}
		for _, san := range strings.Split(entry.NameValue, "\n") {
			san = strings.TrimSpace(san)
			if san != "" {
				sans = append(sans, san)
			}
		}
		certs = append(certs, certResult{
			CommonName: entry.CommonName,
			SANEntries: sans,
			Issuer:     entry.IssuerName,
			NotBefore:  parseCrtshTime(entry.NotBefore),
			NotAfter:   parseCrtshTime(entry.NotAfter),
		})
	}

	out := certTransparencyResult{Certificates: certs}
	b, err := json.Marshal(out)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("failed to serialise result: %s", err.Error()))
	}

	return mcp.NewToolResultText(string(b)), nil
}
