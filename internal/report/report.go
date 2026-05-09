package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/hypothesis"
	"github.com/aslanchik/go-phish/internal/synthesis"
)

// Format renders a completed investigation as a plain-text report.
func Format(inv db.Investigation) string {
	var h hypothesis.Hypothesis
	if len(inv.Hypothesis) > 0 {
		_ = json.Unmarshal(inv.Hypothesis, &h)
	}

	var b strings.Builder

	line := func(width int, label, value string) {
		fmt.Fprintf(&b, "%-*s %s\n", width, label+":", value)
	}

	b.WriteString("=== Phishing Investigation Report ===\n\n")
	line(20, "Investigation ID", inv.ID)
	line(20, "Timestamp", inv.CreatedAt.UTC().Format(time.RFC3339))
	line(20, "URL", inv.URL)
	if inv.FinalURL.Valid && inv.FinalURL.String != inv.URL {
		line(20, "Final URL", inv.FinalURL.String)
	}
	b.WriteString("\n")

	if len(inv.Synthesis) > 0 {
		var s synthesis.Result
		if err := json.Unmarshal(inv.Synthesis, &s); err == nil {
			writeSynthesis(&b, s)
			writeHypothesisRef(&b, h)
			return b.String()
		}
	}

	// Fallback: hypothesis-only (pre-synthesis investigations).
	b.WriteString("--- Hypothesis ---\n\n")
	line(20, "Brand", h.Brand)
	line(20, "Targeted action", h.TargetedAction)
	line(20, "Confidence", h.Confidence)
	if h.Reasoning != "" {
		fmt.Fprintf(&b, "%-20s %s\n", "Reasoning:", h.Reasoning)
	}
	b.WriteString("\n")

	return b.String()
}

func writeSynthesis(b *strings.Builder, s synthesis.Result) {
	claim := func(label string, c synthesis.Claim) {
		fmt.Fprintf(b, "%-22s %s  [%s]\n", label+":", c.Value, c.Confidence)
		fmt.Fprintf(b, "  evidence: %s\n\n", c.Evidence)
	}

	b.WriteString("--- Synthesis ---\n\n")
	claim("Verdict", s.Verdict)
	claim("Brand impersonated", s.BrandImpersonated)
	claim("Kit identified", s.KitIdentification)
	claim("Exfil target", s.ExfilTarget)
	claim("Infrastructure", s.InfrastructureNotes)
}

func writeHypothesisRef(b *strings.Builder, h hypothesis.Hypothesis) {
	line := func(label, value string) {
		fmt.Fprintf(b, "%-22s %s\n", label+":", value)
	}
	b.WriteString("--- Phase 2 Hypothesis (for reference) ---\n\n")
	line("Brand", h.Brand)
	line("Targeted action", h.TargetedAction)
	line("Confidence", h.Confidence)
	if h.Reasoning != "" {
		line("Reasoning", h.Reasoning)
	}
	b.WriteString("\n")
}

// Print writes the formatted report to stdout.
func Print(inv db.Investigation) {
	fmt.Fprint(os.Stdout, Format(inv))
}
