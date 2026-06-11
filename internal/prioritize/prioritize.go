// Package prioritize filters and orders changes so alerts carry signal, not noise.
package prioritize

import (
	"strings"

	"github.com/maruftak/reconsentry/internal/diff"
)

// Level converts a config priority string to a numeric threshold.
func Level(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high":
		return diff.High
	case "medium":
		return diff.Medium
	default:
		return diff.Low
	}
}

// Filter keeps only changes at or above the minimum priority. The input order
// (set by diff.Diff: priority desc, then host) is preserved.
func Filter(changes []diff.Change, min int) []diff.Change {
	out := make([]diff.Change, 0, len(changes))
	for _, c := range changes {
		if c.Priority >= min {
			out = append(out, c)
		}
	}
	return out
}
