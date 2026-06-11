// Package prioritize filters and orders changes so alerts carry signal, not noise.
package prioritize

import (
	"sort"
	"strings"

	"github.com/maruftak/newfound/internal/diff"
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

// Filter keeps only changes at or above the minimum priority.
func Filter(changes []diff.Change, min int) []diff.Change {
	out := make([]diff.Change, 0, len(changes))
	for _, c := range changes {
		if c.Priority >= min {
			out = append(out, c)
		}
	}
	return out
}

// Sort orders changes by priority (descending) then host.
func Sort(changes []diff.Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Priority != changes[j].Priority {
			return changes[i].Priority > changes[j].Priority
		}
		return changes[i].Host < changes[j].Host
	})
}
