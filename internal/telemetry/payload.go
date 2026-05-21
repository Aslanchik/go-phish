package telemetry

import "unicode/utf8"

// PayloadThreshold is the maximum byte length of a string attribute emitted inline.
const PayloadThreshold = 32 * 1024

// Truncate cuts s to at most PayloadThreshold bytes at a valid UTF-8 boundary.
// Returns the (possibly truncated) string and whether truncation occurred.
func Truncate(s string) (value string, truncated bool) {
	if len(s) <= PayloadThreshold {
		return s, false
	}
	cut := s[:PayloadThreshold]
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRuneInString(cut)
		// r != RuneError means a valid rune ends here; size != 1 means a valid
		// multi-byte RuneError (U+FFFD encoded as 3 bytes) — both are safe boundaries.
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut, true
}
