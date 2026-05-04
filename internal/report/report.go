package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aslanchik/go-phish/internal/db"
	"github.com/aslanchik/go-phish/internal/hypothesis"
)

// Format renders a completed investigation as a plain-text report.
func Format(inv db.Investigation) string {
	var h hypothesis.Hypothesis
	if len(inv.Hypothesis) > 0 {
		_ = json.Unmarshal(inv.Hypothesis, &h)
	}

	var b strings.Builder
	line := func(label, value string) {
		fmt.Fprintf(&b, "%-20s %s\n", label+":", value)
	}

	b.WriteString("=== Phishing Investigation Report ===\n\n")
	line("Investigation ID", inv.ID)
	line("Timestamp", inv.CreatedAt.UTC().Format(time.RFC3339))
	line("URL", inv.URL)
	if inv.FinalURL.Valid && inv.FinalURL.String != inv.URL {
		line("Final URL", inv.FinalURL.String)
	}
	b.WriteString("\n--- Hypothesis ---\n\n")
	line("Brand", h.Brand)
	line("Targeted action", h.TargetedAction)
	line("Confidence", h.Confidence)
	if h.Reasoning != "" {
		fmt.Fprintf(&b, "%-20s %s\n", "Reasoning:", h.Reasoning)
	}
	b.WriteString("\n")

	return b.String()
}

// Print writes the formatted report to stdout.
func Print(inv db.Investigation) {
	fmt.Fprint(os.Stdout, Format(inv))
}
