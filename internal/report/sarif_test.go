package report

import (
	"encoding/json"
	"testing"

	"github.com/maruftak/reconsentry/internal/diff"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return m
}

func TestSARIFStructure(t *testing.T) {
	b, err := SARIF([]ScopeChanges{{
		Scope: "prog",
		Changes: []diff.Change{
			{Kind: diff.NewHost, Host: "a.example.com", Detail: "new live host", Priority: diff.High},
			{Kind: diff.NewTech, Host: "b.example.com", Detail: "nginx", Priority: diff.Low},
		},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := decode(t, b)

	if m["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", m["version"])
	}
	runs := m["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(runs))
	}
	run := runs[0].(map[string]any)
	results := run["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}

	r0 := results[0].(map[string]any)
	if r0["ruleId"] != "NEW_HOST" {
		t.Errorf("ruleId = %v, want NEW_HOST", r0["ruleId"])
	}
	if r0["level"] != "error" {
		t.Errorf("high priority should map to error, got %v", r0["level"])
	}
	loc := r0["locations"].([]any)[0].(map[string]any)
	uri := loc["physicalLocation"].(map[string]any)["artifactLocation"].(map[string]any)["uri"]
	if uri != "https://a.example.com" {
		t.Errorf("artifact uri = %v, want https://a.example.com", uri)
	}

	r1 := results[1].(map[string]any)
	if r1["level"] != "note" {
		t.Errorf("low priority should map to note, got %v", r1["level"])
	}
}

func TestSARIFLevelMapping(t *testing.T) {
	cases := map[int]string{diff.High: "error", diff.Medium: "warning", diff.Low: "note", 99: "note"}
	for prio, want := range cases {
		if got := levelFor(prio); got != want {
			t.Errorf("levelFor(%d) = %q, want %q", prio, got, want)
		}
	}
}

func TestSARIFDedupsRules(t *testing.T) {
	b, _ := SARIF([]ScopeChanges{{
		Scope: "prog",
		Changes: []diff.Change{
			{Kind: diff.NewHost, Host: "a.example.com", Priority: diff.High},
			{Kind: diff.NewHost, Host: "b.example.com", Priority: diff.High},
		},
	}})
	run := decode(t, b)["runs"].([]any)[0].(map[string]any)
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 {
		t.Errorf("repeated kind should yield one rule, got %d", len(rules))
	}
}

func TestSARIFOneRunPerScope(t *testing.T) {
	b, _ := SARIF([]ScopeChanges{
		{Scope: "a", Changes: []diff.Change{{Kind: diff.NewHost, Host: "x.a.com", Priority: diff.High}}},
		{Scope: "b", Changes: nil},
	})
	runs := decode(t, b)["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("want one run per scope (2), got %d", len(runs))
	}
}

func TestSARIFEmptyIsValid(t *testing.T) {
	b, err := SARIF(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := decode(t, b)
	if _, ok := m["runs"]; !ok {
		t.Error("empty report should still carry a runs array")
	}
}
