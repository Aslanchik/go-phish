package telemetry

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	t.Run("below threshold", func(t *testing.T) {
		s := strings.Repeat("a", PayloadThreshold-1)
		v, trunc := Truncate(s)
		if trunc {
			t.Error("expected no truncation")
		}
		if v != s {
			t.Error("expected value unchanged")
		}
	})

	t.Run("exactly at threshold", func(t *testing.T) {
		s := strings.Repeat("a", PayloadThreshold)
		v, trunc := Truncate(s)
		if trunc {
			t.Error("expected no truncation at exactly threshold")
		}
		if v != s {
			t.Error("expected value unchanged")
		}
	})

	t.Run("ASCII above threshold", func(t *testing.T) {
		s := strings.Repeat("a", PayloadThreshold+100)
		v, trunc := Truncate(s)
		if !trunc {
			t.Error("expected truncation")
		}
		if len(v) != PayloadThreshold {
			t.Errorf("expected length %d, got %d", PayloadThreshold, len(v))
		}
		if !utf8.ValidString(v) {
			t.Error("truncated string is not valid UTF-8")
		}
	})

	t.Run("mid-rune truncation produces valid UTF-8", func(t *testing.T) {
		// Build a string where the truncation point lands inside a multi-byte rune.
		// Pad with ASCII to reach threshold-1, then append a 3-byte UTF-8 rune (€ = 0xE2 0x80 0xAC).
		// The truncation at PayloadThreshold bytes will cut inside €.
		base := strings.Repeat("a", PayloadThreshold-1)
		s := base + "€" + strings.Repeat("b", 10)
		v, trunc := Truncate(s)
		if !trunc {
			t.Error("expected truncation")
		}
		if !utf8.ValidString(v) {
			t.Errorf("truncated string is not valid UTF-8: %q", v[len(v)-4:])
		}
		// The € starts at PayloadThreshold-1 and is 3 bytes; the cut at
		// PayloadThreshold includes the first byte of €, which is invalid alone.
		// Truncate must walk back to the last valid boundary (PayloadThreshold-1).
		if len(v) != PayloadThreshold-1 {
			t.Errorf("expected length %d, got %d", PayloadThreshold-1, len(v))
		}
	})
}
