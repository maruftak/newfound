package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/runner"
)

func TestPickScope(t *testing.T) {
	scopes := []*config.Config{
		{Name: "alpha"},
		{Name: "beta"},
	}

	got, err := pickScope([]*config.Config{{Name: "solo"}}, "")
	if err != nil {
		t.Fatalf("single unnamed scope should be selected: %v", err)
	}
	if got.Name != "solo" {
		t.Fatalf("selected scope = %q, want solo", got.Name)
	}

	got, err = pickScope(scopes, "beta")
	if err != nil {
		t.Fatalf("named scope should be selected: %v", err)
	}
	if got.Name != "beta" {
		t.Fatalf("selected scope = %q, want beta", got.Name)
	}

	if _, err := pickScope(scopes, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing scope should return not-found error, got %v", err)
	}
	if _, err := pickScope(scopes, ""); err == nil || !strings.Contains(err.Error(), "pass --scope") {
		t.Fatalf("multi-scope config without name should require --scope, got %v", err)
	}
}

func TestPrintJSONShape(t *testing.T) {
	out := captureStdout(t, func() {
		printJSON("prog", &runner.Result{
			RunID:      42,
			FirstRun:   true,
			Assets:     nil,
			Changes:    nil,
			Truncated:  0,
			Findings:   nil,
			NotifyErrs: nil,
		})
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("printJSON wrote invalid JSON %q: %v", out, err)
	}
	if got["scope"] != "prog" || got["run_id"] != float64(42) || got["first_run"] != true {
		t.Fatalf("unexpected JSON shape: %v", got)
	}
	if changes, ok := got["changes"].([]any); !ok || len(changes) != 0 {
		t.Fatalf("changes should be an empty array, got %#v", got["changes"])
	}
}

func TestPrintJSONIncludesChanges(t *testing.T) {
	out := captureStdout(t, func() {
		printJSON("prog", &runner.Result{
			RunID: 7,
			Changes: []diff.Change{
				{Kind: diff.NewHost, Host: "a.example.com", Detail: "new live host", Priority: diff.High},
			},
		})
	})

	if !strings.Contains(out, `"changes":[`) || !strings.Contains(out, `"NEW_HOST"`) {
		t.Fatalf("JSON should include changes, got %q", out)
	}
}

func TestCommandRequiredConfigFlags(t *testing.T) {
	if got := cmdRun(nil); got != 2 {
		t.Fatalf("cmdRun without --config = %d, want 2", got)
	}
	if got := cmdAssets(nil); got != 2 {
		t.Fatalf("cmdAssets without --config = %d, want 2", got)
	}
	if got := cmdHistory(nil); got != 2 {
		t.Fatalf("cmdHistory without --config = %d, want 2", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
