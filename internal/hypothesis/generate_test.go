package hypothesis_test

import (
	"encoding/json"
	"testing"

	"github.com/aslanchik/go-phish/internal/hypothesis"
)

func TestErrNoToolCall_Error(t *testing.T) {
	err := hypothesis.ErrNoToolCall{}
	if err.Error() == "" {
		t.Error("ErrNoToolCall.Error() returned empty string")
	}
}

func TestRecordHypothesisSchema_RequiredFields(t *testing.T) {
	// Verify the Hypothesis struct can round-trip the four required fields.
	input := `{
		"brand":           "PayPal",
		"targeted_action": "credential_theft",
		"confidence":      "high",
		"reasoning":       "Login form posts to evil.example.com, logo matches PayPal branding."
	}`
	var h hypothesis.Hypothesis
	if err := json.Unmarshal([]byte(input), &h); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if h.Brand != "PayPal" {
		t.Errorf("brand: got %q, want %q", h.Brand, "PayPal")
	}
	if h.TargetedAction != "credential_theft" {
		t.Errorf("targeted_action: got %q, want %q", h.TargetedAction, "credential_theft")
	}
	if h.Confidence != "high" {
		t.Errorf("confidence: got %q, want %q", h.Confidence, "high")
	}
	if h.Reasoning == "" {
		t.Error("reasoning should not be empty")
	}
}
