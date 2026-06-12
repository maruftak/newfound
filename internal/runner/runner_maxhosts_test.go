package runner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/model"
	"github.com/maruftak/reconsentry/internal/store"
)

func TestCapHosts(t *testing.T) {
	targets := []string{"root.com"}
	cases := []struct {
		name     string
		hosts    []string
		max      int
		wantLen  int
		wantDrop int
	}{
		{"no limit", []string{"a.com", "b.com"}, 0, 2, 0},
		{"under limit", []string{"a.com", "b.com"}, 5, 2, 0},
		{"caps extras", []string{"root.com", "c.com", "a.com", "b.com"}, 2, 2, 2},
		{"keeps seed over cap", []string{"root.com", "a.com"}, 1, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, drop := capHosts(targets, tc.hosts, tc.max)
			if len(got) != tc.wantLen || drop != tc.wantDrop {
				t.Fatalf("capHosts(%v, %d) = %v drop %d; want len %d drop %d",
					tc.hosts, tc.max, got, drop, tc.wantLen, tc.wantDrop)
			}
		})
	}

	// Seed is always retained even when discovered extras would fill the cap.
	got, _ := capHosts(targets, []string{"z.com", "root.com", "a.com"}, 2)
	seedKept := false
	for _, h := range got {
		if h == "root.com" {
			seedKept = true
		}
	}
	if !seedKept {
		t.Errorf("seed target must always be kept, got %v", got)
	}

	// Extras are capped deterministically: lowest hostnames first.
	got, _ = capHosts(nil, []string{"c.com", "a.com", "b.com"}, 2)
	if len(got) != 2 || got[0] != "a.com" || got[1] != "b.com" {
		t.Errorf("extras should cap to lowest names first, got %v", got)
	}
}

func TestPipelineCapsProbeToMaxHosts(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "p", Targets: []string{"root.com"}, MinPriority: "low"}
	discover := func(_ context.Context, _ []string) ([]string, error) {
		return []string{"a.root.com", "b.root.com", "c.root.com", "d.root.com"}, nil
	}
	probedCount := -1
	probe := func(_ context.Context, h []string) ([]model.Asset, error) {
		probedCount = len(h)
		out := make([]model.Asset, 0, len(h))
		for _, x := range h {
			out = append(out, model.Asset{Host: x, Alive: true, Status: 200})
		}
		return out, nil
	}

	p := &Pipeline{Store: st, Discover: discover, Probe: probe, MaxHosts: 2}
	res, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if probedCount != 2 {
		t.Fatalf("MaxHosts=2 must cap the probe to 2 hosts, probed %d", probedCount)
	}
	// 1 seed + 4 discovered = 5 hosts, capped to 2 -> 3 dropped.
	if res.Truncated != 3 {
		t.Errorf("expected Truncated=3, got %d", res.Truncated)
	}
}

func TestPipelineProbeKillHintsAtLevers(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "p", Targets: []string{"root.com"}, MinPriority: "low"}
	discover := func(_ context.Context, _ []string) ([]string, error) { return []string{"a.root.com"}, nil }
	probe := func(_ context.Context, _ []string) ([]model.Asset, error) {
		return nil, errors.New("httpx: signal: killed")
	}

	p := &Pipeline{Store: st, Discover: discover, Probe: probe}
	_, err = p.Run(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "--max-hosts") {
		t.Fatalf("a killed probe should hint at --timeout/--max-hosts, got %v", err)
	}
}
