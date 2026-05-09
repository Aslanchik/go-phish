package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
	"github.com/mark3labs/mcp-go/mcp"
)

const whoisTimeout = 10 * time.Second

type whoisResult struct {
	Registrar      string `json:"registrar"`
	RegisteredAt   string `json:"registered_at"`
	ExpiresAt      string `json:"expires_at"`
	RegistrantOrg  string `json:"registrant_org"`
	Raw            string `json:"raw"`
	Error          string `json:"error,omitempty"`
}

func whoisErrorResult(reason string) (*mcp.CallToolResult, error) {
	b, _ := json.Marshal(whoisResult{Error: reason})
	return mcp.NewToolResultText(string(b)), nil
}

func whoisHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil || strings.TrimSpace(domain) == "" {
		return whoisErrorResult("missing or empty 'domain' argument")
	}
	domain = strings.TrimSpace(domain)

	client := whois.NewClient().SetTimeout(whoisTimeout)
	raw, err := client.Whois(domain)
	if err != nil {
		return whoisErrorResult("whois query failed: " + err.Error())
	}

	result := whoisResult{Raw: raw}

	parsed, err := whoisparser.Parse(raw)
	if err == nil {
		if parsed.Registrar != nil {
			result.Registrar = parsed.Registrar.Name
		}
		if parsed.Domain != nil {
			if parsed.Domain.CreatedDateInTime != nil {
				result.RegisteredAt = parsed.Domain.CreatedDateInTime.UTC().Format(time.RFC3339)
			}
			if parsed.Domain.ExpirationDateInTime != nil {
				result.ExpiresAt = parsed.Domain.ExpirationDateInTime.UTC().Format(time.RFC3339)
			}
		}
		if parsed.Registrant != nil {
			result.RegistrantOrg = parsed.Registrant.Organization
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		return whoisErrorResult("failed to serialise result: " + err.Error())
	}
	return mcp.NewToolResultText(string(b)), nil
}
