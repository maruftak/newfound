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

// In passive mode the pipeline must never actively probe, scan, or crawl: it
// diffs on discovered subdomains alone (NEW_HOST / HOST_GONE).
func TestPipelinePassiveSkipsProbeScanCrawl(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "prog", Targets: []string{"example.com"}, MinPriority: "low", Passive: true}

	current := []string{"a.example.com"}
	discover := func(_ context.Context, _ []string) ([]string, error) { return current, nil }

	probed := false
	probe := func(_ context.Context, _ []string) ([]model.Asset, error) {
		probed = true
		return nil, nil
	}
	scanned := false
	scanner := func(_ context.Context, _ []string) ([]model.Finding, error) {
		scanned = true
		return nil, nil
	}
	crawled := false
	crawler := func(_ context.Context, _ []string) ([]model.Endpoint, error) {
		crawled = true
		return nil, nil
	}

	p := &Pipeline{Store: st, Discover: discover, Probe: probe, Scanner: scanner, Crawler: crawler}

	// Baseline.
	if _, err := p.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	// A new subdomain appears and the old one disappears.
	current = []string{"b.example.com"}
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if probed {
		t.Error("passive scope must not call Probe (no active httpx)")
	}
	if scanned {
		t.Error("passive scope must not call Scanner (no active nuclei)")
	}
	if crawled {
		t.Error("passive scope must not call Crawler (no active katana)")
	}

	// Only discovery-level changes: NEW_HOST b and HOST_GONE a.
	kinds := map[diff.Kind]string{}
	for _, c := range r2.Changes {
		kinds[c.Kind] = c.Host
	}
	if kinds[diff.NewHost] != "b.example.com" {
		t.Errorf("expected NEW_HOST b.example.com, got changes %+v", r2.Changes)
	}
	if kinds[diff.HostGone] != "a.example.com" {
		t.Errorf("expected HOST_GONE a.example.com, got changes %+v", r2.Changes)
	}
	for _, c := range r2.Changes {
		if c.Kind != diff.NewHost && c.Kind != diff.HostGone {
			t.Errorf("passive diff should only yield NEW_HOST/HOST_GONE, got %s", c.Kind)
		}
	}
}
