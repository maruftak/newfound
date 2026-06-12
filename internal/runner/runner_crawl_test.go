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

func TestPipelineCrawlEndpointDiff(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Name: "prog", Targets: []string{"example.com"}, MinPriority: "low"}

	discover := func(_ context.Context, _ []string) ([]string, error) { return []string{"a.example.com"}, nil }
	probe := func(_ context.Context, h []string) ([]model.Asset, error) {
		out := make([]model.Asset, 0, len(h))
		for _, x := range h {
			out = append(out, model.Asset{Host: x, Alive: true, Status: 200})
		}
		return out, nil
	}

	var urls []string
	crawler := func(_ context.Context, hosts []string) ([]model.Endpoint, error) {
		eps := make([]model.Endpoint, 0, len(urls))
		for _, u := range urls {
			eps = append(eps, model.Endpoint{Host: hosts[0], URL: u})
		}
		return eps, nil
	}

	p := &Pipeline{Store: st, Discover: discover, Probe: probe, Crawler: crawler}

	// First crawl is a baseline: no NEW_ENDPOINT even though it is run 1.
	urls = []string{"https://a.example.com/", "https://a.example.com/login"}
	r1, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r1.Changes {
		if c.Kind == diff.NewEndpoint {
			t.Fatalf("baseline crawl must not emit NEW_ENDPOINT: %+v", r1.Changes)
		}
	}
	if len(r1.Endpoints) != 2 {
		t.Fatalf("want 2 endpoints recorded on baseline, got %d", len(r1.Endpoints))
	}

	// A new URL appears -> exactly one NEW_ENDPOINT.
	urls = []string{
		"https://a.example.com/",
		"https://a.example.com/login",
		"https://a.example.com/admin?debug=1",
	}
	r2, err := p.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var got string
	for _, c := range r2.Changes {
		if c.Kind == diff.NewEndpoint {
			n++
			got = c.Detail
		}
	}
	if n != 1 || got != "https://a.example.com/admin?debug=1" {
		t.Fatalf("want one NEW_ENDPOINT for the new url, got %d (%q) in %+v", n, got, r2.Changes)
	}
}
