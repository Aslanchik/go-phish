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
