package collect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestParseOTX(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		target string
		want   []string
	}{
		{
			name:   "happy path extracts hostnames",
			body:   `{"passive_dns":[{"hostname":"a.example.com"},{"hostname":"b.example.com"}]}`,
			target: "example.com",
			want:   []string{"a.example.com", "b.example.com"},
		},
		{
			name:   "dedups and lowercases",
			body:   `{"passive_dns":[{"hostname":"A.example.com"},{"hostname":"a.example.com"}]}`,
			target: "example.com",
			want:   []string{"a.example.com"},
		},
		{
			name:   "drops out-of-scope co-resolving names",
			body:   `{"passive_dns":[{"hostname":"a.example.com"},{"hostname":"cdn.akamai.net"}]}`,
			target: "example.com",
			want:   []string{"a.example.com"},
		},
		{
			name:   "matches apex domain",
			body:   `{"passive_dns":[{"hostname":"example.com"}]}`,
			target: "example.com",
			want:   []string{"example.com"},
		},
		{
			name:   "empty list yields nothing",
			body:   `{"passive_dns":[]}`,
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
			got := parseOTX([]byte(tt.body), tt.target)
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

func TestOTXMergesAcrossTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/domain/example.com/"):
			w.Write([]byte(`{"passive_dns":[{"hostname":"a.example.com"}]}`))
		case strings.Contains(r.URL.Path, "/domain/example.org/"):
			w.Write([]byte(`{"passive_dns":[{"hostname":"b.example.org"}]}`))
		default:
			w.Write([]byte(`{"passive_dns":[]}`))
		}
	}))
	defer srv.Close()
	defer stubHTTPGetPath(srv.URL)()

	got, err := OTX(context.Background(), []string{"example.com", "example.org"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"a.example.com", "b.example.org"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOTXFailsSoftOnPartialError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/domain/dead.com/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"passive_dns":[{"hostname":"a.example.com"}]}`))
	}))
	defer srv.Close()
	defer stubHTTPGetPath(srv.URL)()

	got, err := OTX(context.Background(), []string{"dead.com", "example.com"})
	if err != nil {
		t.Fatalf("a dead target should not fail the run: %v", err)
	}
	if len(got) != 1 || got[0] != "a.example.com" {
		t.Fatalf("got %v, want [a.example.com]", got)
	}
}

func TestOTXErrorsWhenAllTargetsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	defer stubHTTPGetPath(srv.URL)()

	if _, err := OTX(context.Background(), []string{"example.com"}); err == nil {
		t.Fatal("want error when every lookup fails, got nil")
	}
}

// stubHTTPGetPath routes the package httpGet at base while preserving the
// original request's path and query — needed for path-based endpoints like OTX
// that carry no query string. Returns a restore func.
func stubHTTPGetPath(base string) func() {
	orig := httpGet
	httpGet = func(ctx context.Context, rawURL string) ([]byte, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		dst := base + u.Path
		if u.RawQuery != "" {
			dst += "?" + u.RawQuery
		}
		return orig(ctx, dst)
	}
	return func() { httpGet = orig }
}
