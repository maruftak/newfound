package collect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestParseWayback(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		target string
		want   []string
	}{
		{
			name:   "happy path extracts hosts and skips header",
			body:   `[["original"],["http://a.example.com/x"],["https://b.example.com/y?q=1"]]`,
			target: "example.com",
			want:   []string{"a.example.com", "b.example.com"},
		},
		{
			name:   "dedups repeated hosts across paths",
			body:   `[["original"],["http://a.example.com/1"],["http://a.example.com/2"]]`,
			target: "example.com",
			want:   []string{"a.example.com"},
		},
		{
			name:   "strips port and lowercases",
			body:   `[["original"],["https://API.Example.com:8443/v1"]]`,
			target: "example.com",
			want:   []string{"api.example.com"},
		},
		{
			name:   "handles query-only url with no path",
			body:   `[["original"],["http://c.example.com?utm=1"]]`,
			target: "example.com",
			want:   []string{"c.example.com"},
		},
		{
			name:   "drops out-of-scope hosts",
			body:   `[["original"],["http://a.example.com/x"],["http://evil.com/y"]]`,
			target: "example.com",
			want:   []string{"a.example.com"},
		},
		{
			name:   "header only yields nothing",
			body:   `[["original"]]`,
			target: "example.com",
			want:   nil,
		},
		{
			name:   "empty array yields nothing",
			body:   `[]`,
			target: "example.com",
			want:   nil,
		},
		{
			name:   "malformed json yields nothing",
			body:   `not json`,
			target: "example.com",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWayback([]byte(tt.body), tt.target)
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestWaybackMergesAcrossTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("url") {
		case "*.example.com":
			w.Write([]byte(`[["original"],["http://a.example.com/x"]]`))
		case "*.example.org":
			w.Write([]byte(`[["original"],["http://b.example.org/y"]]`))
		default:
			w.Write([]byte(`[["original"]]`))
		}
	}))
	defer srv.Close()
	defer stubHTTPGet(srv.URL)()

	got, err := Wayback(context.Background(), []string{"example.com", "example.org"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"a.example.com", "b.example.org"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWaybackFailsSoftOnPartialError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") == "*.dead.com" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`[["original"],["http://a.example.com/x"]]`))
	}))
	defer srv.Close()
	defer stubHTTPGet(srv.URL)()

	got, err := Wayback(context.Background(), []string{"dead.com", "example.com"})
	if err != nil {
		t.Fatalf("a dead target should not fail the run: %v", err)
	}
	if len(got) != 1 || got[0] != "a.example.com" {
		t.Fatalf("got %v, want [a.example.com]", got)
	}
}

func TestWaybackErrorsWhenAllTargetsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	defer stubHTTPGet(srv.URL)()

	if _, err := Wayback(context.Background(), []string{"example.com"}); err == nil {
		t.Fatal("want error when every lookup fails, got nil")
	}
}
