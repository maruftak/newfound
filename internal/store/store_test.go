package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/maruftak/reconsentry/internal/model"
)

func TestSaveAndLatest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// No prior run -> nil, nil.
	got, err := st.LatestAssets("scope1")
	if err != nil || got != nil {
		t.Fatalf("empty scope: want nil,nil; got %v, %v", got, err)
	}

	run1 := []model.Asset{
		{Target: "example.com", Host: "a.example.com", IP: "1.1.1.1", Alive: true, Status: 200,
			Tech: []string{"nginx", "php"}, Title: "Home", Server: "nginx"},
		{Target: "example.com", Host: "b.example.com", Alive: false},
	}
	if _, err := st.SaveRun("scope1", time.Now(), run1); err != nil {
		t.Fatal(err)
	}

	got, err = st.LatestAssets("scope1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 assets, got %d", len(got))
	}

	byHost := map[string]model.Asset{}
	for _, a := range got {
		byHost[a.Host] = a
	}
	if a := byHost["a.example.com"]; !a.Alive || a.Status != 200 || a.IP != "1.1.1.1" || len(a.Tech) != 2 || a.Server != "nginx" {
		t.Errorf("round-trip mismatch for a: %+v", a)
	}
	if b := byHost["b.example.com"]; b.Alive {
		t.Errorf("b should be not alive: %+v", b)
	}

	// A second run must shadow the first for LatestAssets.
	if _, err := st.SaveRun("scope1", time.Now(), []model.Asset{
		{Target: "example.com", Host: "c.example.com", Alive: true, Status: 200},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.LatestAssets("scope1")
	if len(got) != 1 || got[0].Host != "c.example.com" {
		t.Fatalf("latest should be run2 (c only), got %+v", got)
	}

	// Scopes are isolated.
	if other, _ := st.LatestAssets("scope2"); other != nil {
		t.Errorf("scope2 should be empty, got %+v", other)
	}
}

func TestRuns(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if runs, _ := st.Runs("s"); len(runs) != 0 {
		t.Fatalf("empty scope should have no runs, got %d", len(runs))
	}

	t0 := time.Now()
	if _, err := st.SaveRun("s", t0, []model.Asset{{Host: "a.example.com"}, {Host: "b.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveRun("s", t0.Add(time.Hour), []model.Asset{{Host: "a.example.com"}}); err != nil {
		t.Fatal(err)
	}

	runs, err := st.Runs("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}
	if runs[0].ID < runs[1].ID {
		t.Errorf("runs should be newest-first, got ids %d then %d", runs[0].ID, runs[1].ID)
	}
	if runs[0].Assets != 1 || runs[1].Assets != 2 {
		t.Errorf("asset counts wrong: got %d, %d (want 1, 2)", runs[0].Assets, runs[1].Assets)
	}
	if runs[1].StartedAt.IsZero() {
		t.Errorf("started_at should round-trip from the db, got zero time")
	}
}

func TestPrune(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 4; i++ {
		if _, err := st.SaveRun("s", time.Now(), []model.Asset{{Host: "a.example.com"}}); err != nil {
			t.Fatal(err)
		}
	}
	// A different scope that must be left untouched by pruning "s".
	if _, err := st.SaveRun("other", time.Now(), []model.Asset{{Host: "z.example.com"}}); err != nil {
		t.Fatal(err)
	}

	if err := st.Prune("s", 2); err != nil {
		t.Fatal(err)
	}
	runs, err := st.Runs("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("after Prune(2), want 2 runs, got %d", len(runs))
	}
	if latest, err := st.LatestAssets("s"); err != nil || len(latest) != 1 {
		t.Fatalf("latest assets after prune: got %v, %v", latest, err)
	}

	// keep <= 0 is a no-op; other scopes untouched.
	if err := st.Prune("s", 0); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.Runs("s"); len(r) != 2 {
		t.Errorf("Prune(0) should be a no-op, got %d runs", len(r))
	}
	if r, _ := st.Runs("other"); len(r) != 1 {
		t.Errorf("pruning scope s must not touch scope other, got %d runs", len(r))
	}
}
