package model

import "testing"

func TestTrimInvisible(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM, U+FEFF

	cases := map[string]string{
		bom + "example.com": "example.com",
		"  example.com  ":   "example.com",
		bom + " a.com ":     "a.com",
		"a.com" + bom:       "a.com",
		"plain.com":         "plain.com",
	}
	for in, want := range cases {
		if got := TrimInvisible(in); got != want {
			t.Errorf("TrimInvisible(%q) = %q, want %q", in, got, want)
		}
	}
}
