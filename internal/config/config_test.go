package config

import "testing"

func TestParseValid(t *testing.T) {
	y := []byte(`
name: prog
targets:
  - Example.com
  - example.org
exclude:
  - dev.example.com
min_priority: high
notify:
  webhooks: ["https://my.endpoint/hook"]
  slack: ["https://hooks.slack.com/services/x"]
  discord: ["https://discord.com/api/webhooks/y"]
`)
	c, err := Parse(y)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "prog" || len(c.Targets) != 2 {
		t.Fatalf("unexpected config: %+v", c)
	}
	if got := c.Notify.Endpoints(); len(got) != 3 {
		t.Errorf("want 3 notify endpoints, got %d: %v", len(got), got)
	}
	if c.Targets[0] != "example.com" {
		t.Errorf("target should be lowercased, got %q", c.Targets[0])
	}
	if !c.Excluded("api.dev.example.com") {
		t.Errorf("api.dev.example.com should be excluded by suffix")
	}
	if c.Excluded("example.com") {
		t.Errorf("root example.com should not be excluded")
	}
}

func TestParseDefaultsPriority(t *testing.T) {
	c, err := Parse([]byte("name: x\ntargets: [a.com]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.MinPriority != "low" {
		t.Errorf("default min_priority should be low, got %q", c.MinPriority)
	}
}

func TestParseStripsBOM(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM (e.g. Notepad-saved YAML)
	// BOM at the start of the file and embedded in a target value.
	y := []byte(bom + "name: x\ntargets:\n  - " + bom + "Example.com\n")
	c, err := Parse(y)
	if err != nil {
		t.Fatalf("BOM-prefixed config should parse, got %v", err)
	}
	if len(c.Targets) != 1 || c.Targets[0] != "example.com" {
		t.Fatalf("BOM not stripped: targets = %q", c.Targets)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"no name":       "targets: [a.com]",
		"no targets":    "name: x",
		"scheme target": "name: x\ntargets: [\"https://a.com\"]",
		"path target":   "name: x\ntargets: [\"a.com/app\"]",
		"bad priority":  "name: x\ntargets: [a.com]\nmin_priority: urgent",
		"notify url":    "name: x\ntargets: [a.com]\nnotify:\n  slack: [\"not-a-url\"]",
	}
	for name, y := range cases {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseAllMultiScope(t *testing.T) {
	y := []byte(`
scopes:
  - name: prog1
    targets: [a.com]
  - name: prog2
    targets: [b.com]
    min_priority: high
`)
	scopes, err := ParseAll(y)
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 2 {
		t.Fatalf("want 2 scopes, got %d", len(scopes))
	}
	if scopes[0].Name != "prog1" || scopes[1].Name != "prog2" {
		t.Errorf("names: %q, %q", scopes[0].Name, scopes[1].Name)
	}
	if scopes[1].MinPriority != "high" {
		t.Errorf("prog2 min_priority should be high, got %q", scopes[1].MinPriority)
	}
}

func TestParseAllSingleScope(t *testing.T) {
	scopes, err := ParseAll([]byte("name: solo\ntargets: [a.com]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 1 || scopes[0].Name != "solo" {
		t.Fatalf("want 1 scope named solo, got %+v", scopes)
	}
}

func TestParseAllRejectsDuplicateNames(t *testing.T) {
	y := []byte("scopes:\n  - name: dup\n    targets: [a.com]\n  - name: dup\n    targets: [b.com]\n")
	if _, err := ParseAll(y); err == nil {
		t.Error("duplicate scope names should error")
	}
}

func TestParseRejectsMultiScope(t *testing.T) {
	y := []byte("scopes:\n  - name: a\n    targets: [a.com]\n  - name: b\n    targets: [b.com]\n")
	if _, err := Parse(y); err == nil {
		t.Error("single-scope Parse should reject a multi-scope file")
	}
}
