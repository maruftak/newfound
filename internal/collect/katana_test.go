package collect

import "testing"

func TestParseKatana(t *testing.T) {
	in := []byte("https://a.example.com/\n" +
		"https://a.example.com/login?next=/\n" +
		"https://a.example.com/login?next=/\n" + // duplicate
		"https://a.example.com/x#frag\n" + // fragment stripped
		"\n")
	got := parseKatana(in)
	if len(got) != 3 {
		t.Fatalf("want 3 unique endpoints, got %d: %+v", len(got), got)
	}
	for _, e := range got {
		if e.Host != "a.example.com" {
			t.Errorf("host should derive from url, got %q for %q", e.Host, e.URL)
		}
		if e.URL == "https://a.example.com/x#frag" {
			t.Errorf("fragment should have been stripped: %q", e.URL)
		}
	}
}
