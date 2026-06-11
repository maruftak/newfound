package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/model"
	"github.com/maruftak/reconsentry/internal/notify"
	"github.com/maruftak/reconsentry/internal/store"
)

type fakeNotifier struct{ got [][]diff.Change }

func (f *fakeNotifier) Notify(_ context.Context, _ string, changes []diff.Change) error {
	f.got = append(f.got, changes)
	return nil
}

func TestPipelineDetectsNewHostEndToEnd(t *testing.T) {
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
	notifier := &fakeNotifier{}
	p := &Pipeline{Store: st, Discover: discover, Probe: probe, Notifiers: []notify.Notifier{notifier}}

	r1, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !r1.FirstRun {
		t.Fatal("run1 should be the baseline run")
	}
	if len(r1.Changes) != 0 {
		t.Fatalf("baseline run should report no changes, got %+v", r1.Changes)
	}

	// A new asset appears between runs.
	current = []string{"a.example.com", "b.example.com"}
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r2.FirstRun {
		t.Fatal("run2 must not be a first run")
	}
	if len(r2.Changes) != 1 || r2.Changes[0].Kind != diff.NewHost || r2.Changes[0].Host != "b.example.com" {
		t.Fatalf("run2 expected NEW_HOST b.example.com, got %+v", r2.Changes)
	}
	if r2.Changes[0].Target != "example.com" {
		t.Errorf("target should be matched to in-scope root, got %q", r2.Changes[0].Target)
	}

	last := notifier.got[len(notifier.got)-1]
	if len(last) != 1 {
		t.Errorf("notifier should have received the 1 change, got %d", len(last))
	}
}

func TestPipelineExcludesAndFiltersPriority(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// min_priority high should drop low-priority NEW_TECH changes.
	cfg := &config.Config{
		Name:        "prog",
		Targets:     []string{"example.com"},
		Exclude:     []string{"skip.example.com"},
		MinPriority: "high",
	}

	current := []string{"a.example.com"}
	tech := []string{"nginx"}
	discover := func(_ context.Context, _ []string) ([]string, error) { return current, nil }
	probe := func(_ context.Context, h []string) ([]model.Asset, error) {
		out := make([]model.Asset, 0, len(h))
		for _, x := range h {
			out = append(out, model.Asset{Host: x, Alive: true, Status: 200, Tech: tech})
		}
		return out, nil
	}
	p := &Pipeline{Store: st, Discover: discover, Probe: probe}

	if _, err := p.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	// Same hosts, but a new tech appears (low priority) and an excluded host shows up.
	current = []string{"a.example.com", "skip.example.com"}
	tech = []string{"nginx", "php"}
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Changes) != 0 {
		t.Fatalf("high min_priority should filter low NEW_TECH and excluded host, got %+v", r2.Changes)
	}
}

func TestPipelineDropsIPChangeUnlessTracked(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "prog", Targets: []string{"a.example.com"}, MinPriority: "low"}

	ip := "1.1.1.1"
	discover := func(_ context.Context, _ []string) ([]string, error) { return nil, nil }
	probe := func(_ context.Context, h []string) ([]model.Asset, error) {
		out := make([]model.Asset, 0, len(h))
		for _, x := range h {
			out = append(out, model.Asset{Host: x, Alive: true, Status: 200, IP: ip})
		}
		return out, nil
	}
	p := &Pipeline{Store: st, Discover: discover, Probe: probe}
	if _, err := p.Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	// IP rotates — must be dropped by default (CDN noise).
	ip = "2.2.2.2"
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Changes) != 0 {
		t.Fatalf("IP_CHANGE should be suppressed by default, got %+v", r2.Changes)
	}

	// With tracking enabled, the change surfaces.
	cfg.TrackIP = true
	ip = "3.3.3.3"
	r3, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r3.Changes) != 1 || r3.Changes[0].Kind != diff.IPChange {
		t.Fatalf("with track_ip enabled, expected one IP_CHANGE, got %+v", r3.Changes)
	}
}
