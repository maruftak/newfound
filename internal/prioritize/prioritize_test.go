package prioritize

import (
	"testing"

	"github.com/maruftak/reconsentry/internal/diff"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"high", diff.High},
		{"medium", diff.Medium},
		{"low", diff.Low},
		{"HIGH", diff.High},
		{"  Medium  ", diff.Medium},
		{"", diff.Low},         // empty defaults to low
		{"bogus", diff.Low},    // unknown defaults to low
		{"critical", diff.Low}, // not a known level → low
	}
	for _, tt := range tests {
		if got := Level(tt.in); got != tt.want {
			t.Errorf("Level(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFilter(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.NewHost, Host: "h.example.com", Priority: diff.High},
		{Kind: diff.StatusChange, Host: "m.example.com", Priority: diff.Medium},
		{Kind: diff.NewTech, Host: "l.example.com", Priority: diff.Low},
	}

	tests := []struct {
		name string
		min  int
		want []string // hosts expected, in order
	}{
		{"low keeps all", diff.Low, []string{"h.example.com", "m.example.com", "l.example.com"}},
		{"medium drops low", diff.Medium, []string{"h.example.com", "m.example.com"}},
		{"high keeps only high", diff.High, []string{"h.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filter(changes, tt.min)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d changes, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, h := range tt.want {
				if got[i].Host != h {
					t.Errorf("position %d: got %q, want %q (order must be preserved)", i, got[i].Host, h)
				}
			}
		})
	}
}

func TestFilterEmptyInput(t *testing.T) {
	got := Filter(nil, diff.Low)
	if len(got) != 0 {
		t.Errorf("filtering nil should yield empty, got %+v", got)
	}
}

func TestFilterDoesNotMutateInput(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.NewHost, Host: "h.example.com", Priority: diff.High},
		{Kind: diff.NewTech, Host: "l.example.com", Priority: diff.Low},
	}
	_ = Filter(changes, diff.High)
	if len(changes) != 2 || changes[1].Host != "l.example.com" {
		t.Errorf("Filter must not mutate its input, got %+v", changes)
	}
}
