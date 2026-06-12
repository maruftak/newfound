package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/model"
	"github.com/maruftak/reconsentry/internal/store"
)

func TestPipelineScansOnlyNewHosts(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "prog", Targets: []string{"example.com"}, MinPriority: "low"}

	current := []string{"a.example.com"}
	discover := func(_ context.Context, _ []string) ([]string, error) { return current, nil }
	probe := func(_ context.Context, h []string) ([]model.Asset, error) {
		out := make([]model.Asset, 0, len(h))
		for _, x := range h {
			out = append(out, model.Asset{Host: x, Alive: true, Status: 200})
		}
		return out, nil
	}

	var scanned []string
	scanner := func(_ context.Context, hosts []string) ([]model.Finding, error) {
		scanned = append(scanned, hosts...)
		return []model.Finding{{Host: hosts[0], TemplateID: "cve-x", Name: "Bad Thing", Severity: "high"}}, nil
	}

	p := &Pipeline{Store: st, Discover: discover, Probe: probe, Scanner: scanner}

	// Baseline: nothing is "new" on the first run, so no scan.
	if _, err := p.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 0 {
		t.Fatalf("baseline run must not scan, scanned %v", scanned)
	}

	// A new host appears: it is scanned, and the finding surfaces as a
	// VULN_FOUND change so it flows to the notifiers.
	current = []string{"a.example.com", "b.example.com"}
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 1 || scanned[0] != "b.example.com" {
		t.Fatalf("should scan only the new host, scanned %v", scanned)
	}

	var vuln *diff.Change
	for i := range r2.Changes {
		if r2.Changes[i].Kind == diff.VulnFound {
			vuln = &r2.Changes[i]
		}
	}
	if vuln == nil {
		t.Fatalf("expected a VULN_FOUND change, got %+v", r2.Changes)
	}
	if vuln.Host != "b.example.com" || len(r2.Findings) != 1 {
		t.Errorf("unexpected vuln/findings: change=%+v findings=%+v", vuln, r2.Findings)
	}
}
