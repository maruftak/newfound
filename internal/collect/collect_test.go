package collect

import "testing"

func TestParseLines(t *testing.T) {
	in := []byte("a.example.com\nb.example.com\n\nA.EXAMPLE.COM\n b.example.com \n")
	got := parseLines(in)
	if len(got) != 2 {
		t.Fatalf("want 2 unique hosts, got %d: %v", len(got), got)
	}
}

func TestParseHttpx(t *testing.T) {
	line := `{"input":"A.example.com","url":"https://a.example.com","status_code":200,"title":"Home","webserver":"nginx","tech":["Nginx","PHP"],"a":["1.2.3.4"]}`
	got := parseHttpx([]byte(line + "\nnot-json\n\n"))
	if len(got) != 1 {
		t.Fatalf("want 1 asset (bad lines skipped), got %d", len(got))
	}
	a := got[0]
	if a.Host != "a.example.com" {
		t.Errorf("host should be cleaned/lowercased, got %q", a.Host)
	}
	if !a.Alive || a.Status != 200 || a.IP != "1.2.3.4" || a.Server != "nginx" || len(a.Tech) != 2 {
		t.Errorf("unexpected asset: %+v", a)
	}
}

func TestParseHttpxTechnologiesField(t *testing.T) {
	line := `{"input":"x.example.com","status_code":404,"technologies":["Apache"]}`
	got := parseHttpx([]byte(line))
	if len(got) != 1 || len(got[0].Tech) != 1 || got[0].Tech[0] != "Apache" {
		t.Fatalf("technologies field not parsed: %+v", got)
	}
}

func TestParseHttpxPrefersHostIP(t *testing.T) {
	line := `{"input":"x.example.com","status_code":200,"host_ip":"9.9.9.9","a":["2.2.2.2","1.1.1.1"]}`
	got := parseHttpx([]byte(line))
	if len(got) != 1 || got[0].IP != "9.9.9.9" {
		t.Fatalf("should prefer stable host_ip, got %+v", got)
	}
}

func TestCleanHostStripsBOM(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if got := cleanHost(bom + "A.COM"); got != "a.com" {
		t.Errorf("cleanHost should strip a leading BOM, got %q", got)
	}
}

func TestCleanHost(t *testing.T) {
	cases := map[string]string{
		"https://a.com/app": "a.com",
		"a.com:8443":        "a.com",
		"  B.COM  ":         "b.com",
		"http://c.com":      "c.com",
	}
	for in, want := range cases {
		if got := cleanHost(in); got != want {
			t.Errorf("cleanHost(%q) = %q, want %q", in, got, want)
		}
	}
}
