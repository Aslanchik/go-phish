package synthesis

import (
	"context"
	"encoding/json"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/aslanchik/go-phish/internal/db"
)

// wellFormedToolUseJSON is a raw content block JSON that the API would return
// when the model calls record_synthesis.
const wellFormedToolUseJSON = `{
	"type": "tool_use",
	"id":   "toolu_01",
	"name": "record_synthesis",
	"input": {
		"brand_impersonated":   {"value": "PayPal",               "confidence": "high",   "evidence": "Logo and domain match PayPal branding"},
		"kit_identification":   {"value": "generic harvester",    "confidence": "medium", "evidence": "analyze_js: POST to external endpoint"},
		"exfil_target":         {"value": "collect.evil.com",     "confidence": "high",   "evidence": "analyze_js: POST https://collect.evil.com/collect.php"},
		"infrastructure_notes": {"value": "Domain 2 days old",    "confidence": "high",   "evidence": "whois_lookup: registered 2026-05-07"},
		"verdict":              {"value": "phishing",             "confidence": "high",   "evidence": "Fresh domain + credential harvesting form + known exfil host"}
	}
}`

func TestParseBlocks_WellFormed(t *testing.T) {
	var block anthropic.ContentBlockUnion
	if err := json.Unmarshal([]byte(wellFormedToolUseJSON), &block); err != nil {
		t.Fatalf("unmarshal content block: %v", err)
	}

	r, err := parseBlocks([]anthropic.ContentBlockUnion{block})
	if err != nil {
		t.Fatalf("parseBlocks: %v", err)
	}

	if r.BrandImpersonated.Value != "PayPal" {
		t.Errorf("brand: got %q, want %q", r.BrandImpersonated.Value, "PayPal")
	}
	if r.BrandImpersonated.Confidence != "high" {
		t.Errorf("brand confidence: got %q, want %q", r.BrandImpersonated.Confidence, "high")
	}
	if r.KitIdentification.Value == "" {
		t.Error("kit_identification.value should not be empty")
	}
	if r.ExfilTarget.Value != "collect.evil.com" {
		t.Errorf("exfil_target: got %q, want %q", r.ExfilTarget.Value, "collect.evil.com")
	}
	if r.InfrastructureNotes.Value == "" {
		t.Error("infrastructure_notes.value should not be empty")
	}
	if r.Verdict.Value != "phishing" {
		t.Errorf("verdict: got %q, want %q", r.Verdict.Value, "phishing")
	}
	// Verify all five claims have non-empty evidence.
	for _, pair := range []struct {
		name     string
		evidence string
	}{
		{"brand_impersonated", r.BrandImpersonated.Evidence},
		{"kit_identification", r.KitIdentification.Evidence},
		{"exfil_target", r.ExfilTarget.Evidence},
		{"infrastructure_notes", r.InfrastructureNotes.Evidence},
		{"verdict", r.Verdict.Evidence},
	} {
		if pair.evidence == "" {
			t.Errorf("%s: evidence should not be empty", pair.name)
		}
	}
}

func TestParseBlocks_NoToolCall(t *testing.T) {
	textBlockJSON := `{"type":"text","text":"I have analysed the page."}`
	var block anthropic.ContentBlockUnion
	if err := json.Unmarshal([]byte(textBlockJSON), &block); err != nil {
		t.Fatalf("unmarshal content block: %v", err)
	}

	_, err := parseBlocks([]anthropic.ContentBlockUnion{block})
	if err == nil {
		t.Fatal("expected error for response with no tool_use block, got nil")
	}
	if err != ErrNoToolCall {
		t.Errorf("got error %v, want ErrNoToolCall", err)
	}
}

func TestParseBlocks_EmptyContent(t *testing.T) {
	_, err := parseBlocks([]anthropic.ContentBlockUnion{})
	if err != ErrNoToolCall {
		t.Errorf("got error %v, want ErrNoToolCall", err)
	}
}

func TestGenerate_NullHypothesis(t *testing.T) {
	inv := db.Investigation{} // Hypothesis is nil json.RawMessage
	_, err := Generate(context.Background(), nil, inv)
	if err == nil {
		t.Fatal("expected error for null hypothesis, got nil")
	}
	if err != ErrNoHypothesis {
		t.Errorf("got error %v, want ErrNoHypothesis", err)
	}
}
