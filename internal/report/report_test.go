package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/report"
)

func TestFormat_ContainsRequiredFields(t *testing.T) {
	hyp, _ := json.Marshal(map[string]string{
		"brand":           "PayPal",
		"targeted_action": "credential_theft",
		"confidence":      "high",
		"reasoning":       "Login form posts to an unrelated domain.",
	})

	inv := db.Investigation{
		ID:         "abc-123",
		URL:        "http://evil.example.com/paypal",
		CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Status:     "complete",
		Hypothesis: hyp,
	}

	out := report.Format(inv)

	checks := []string{
		"abc-123",
		"http://evil.example.com/paypal",
		"PayPal",
		"credential_theft",
		"high",
		"Login form posts to an unrelated domain.",
		"2026-01-02T03:04:05Z",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestFormat_EmptyHypothesis(t *testing.T) {
	inv := db.Investigation{
		ID:        "xyz-456",
		URL:       "http://example.com",
		CreatedAt: time.Now(),
		Status:    "failed",
	}
	out := report.Format(inv)
	if !strings.Contains(out, "xyz-456") {
		t.Error("report missing investigation ID")
	}
}

func TestFormat_WithSynthesis(t *testing.T) {
	hyp, _ := json.Marshal(map[string]string{
		"brand":           "Ledger",
		"targeted_action": "credential_theft",
		"confidence":      "high",
		"reasoning":       "Ledger logo and seed phrase form.",
	})
	synth, _ := json.Marshal(map[string]any{
		"brand_impersonated":   map[string]string{"value": "Ledger", "confidence": "high", "evidence": "analyze_js: Ledger branding in kit source"},
		"kit_identification":   map[string]string{"value": "unknown", "confidence": "low", "evidence": "analyze_js: no recognisable kit signature"},
		"exfil_target":         map[string]string{"value": "collect.evil.com", "confidence": "medium", "evidence": "analyze_js: POST to collect.evil.com"},
		"infrastructure_notes": map[string]string{"value": "Domain 2 days old", "confidence": "high", "evidence": "whois_lookup: registered 2026-05-07"},
		"verdict":              map[string]string{"value": "phishing", "confidence": "high", "evidence": "Fresh domain + credential harvesting"},
	})

	inv := db.Investigation{
		ID:        "syn-001",
		URL:       "http://evil.example.com/ledger",
		CreatedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Status:    "complete",
		Hypothesis: hyp,
		Synthesis:  synth,
	}

	out := report.Format(inv)

	// Synthesis section must be present with all five claims.
	synthChecks := []string{
		"--- Synthesis ---",
		"Verdict:",
		"phishing",
		"[high]",
		"Brand impersonated:",
		"Ledger",
		"Kit identified:",
		"Exfil target:",
		"collect.evil.com",
		"Infrastructure:",
		"Domain 2 days old",
		// Each claim must have an evidence line.
		"evidence: analyze_js: Ledger branding",
		"evidence: Fresh domain",
	}
	for _, want := range synthChecks {
		if !strings.Contains(out, want) {
			t.Errorf("synthesis report missing %q\nfull output:\n%s", want, out)
		}
	}

	// Phase 2 hypothesis reference section must follow.
	if !strings.Contains(out, "--- Phase 2 Hypothesis (for reference) ---") {
		t.Errorf("missing hypothesis reference section\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "Ledger") {
		t.Errorf("hypothesis reference missing brand\nfull output:\n%s", out)
	}
}

func TestFormat_NoSynthesis_UnchangedBehaviour(t *testing.T) {
	hyp, _ := json.Marshal(map[string]string{
		"brand":           "PayPal",
		"targeted_action": "credential_theft",
		"confidence":      "medium",
		"reasoning":       "Looks like PayPal login.",
	})
	inv := db.Investigation{
		ID:         "old-001",
		URL:        "http://evil.example.com/pp",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:     "complete",
		Hypothesis: hyp,
	}

	out := report.Format(inv)

	if strings.Contains(out, "--- Synthesis ---") {
		t.Error("synthesis section should not appear when inv.Synthesis is empty")
	}
	if !strings.Contains(out, "--- Hypothesis ---") {
		t.Error("hypothesis section should appear when inv.Synthesis is empty")
	}
	if !strings.Contains(out, "PayPal") {
		t.Error("hypothesis brand should appear")
	}
}
