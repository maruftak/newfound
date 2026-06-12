package collect

import "testing"

func TestParseNuclei(t *testing.T) {
	line := `{"template-id":"tech-detect","host":"a.example.com","matched-at":"https://a.example.com","info":{"name":"Tech Detect","severity":"info"}}`
	got := parseNuclei([]byte(line + "\nnot-json\n"))
	if len(got) != 1 {
		t.Fatalf("want 1 finding (bad line skipped), got %d", len(got))
	}
	f := got[0]
	if f.Host != "a.example.com" || f.TemplateID != "tech-detect" || f.Severity != "info" || f.Name != "Tech Detect" {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestParseNucleiDerivesHostFromMatchedAt(t *testing.T) {
	line := `{"template-id":"x","matched-at":"https://b.example.com:443/login","info":{"name":"X","severity":"high"}}`
	got := parseNuclei([]byte(line))
	if len(got) != 1 || got[0].Host != "b.example.com" {
		t.Fatalf("host should derive from matched-at, got %+v", got)
	}
}
