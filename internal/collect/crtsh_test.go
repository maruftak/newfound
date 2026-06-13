package collect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestParseCrtSh(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		target string
		want   []string
	}{
		{
			name:   "happy path extracts and dedups names",
			body:   `[{"name_value":"a.example.com\nb.example.com","common_name":"a.example.com"},{"name_value":"c.example.com","common_name":""}]`,
			target: "example.com",
			want:   []string{"a.example.com", "b.example.com", "c.example.com"},
		},
		{
			name:   "unwraps wildcard to base domain",
			body:   `[{"name_value":"*.example.com","common_name":"*.example.com"}]`,
			target: "example.com",
			want:   []string{"example.com"},
		},
		{
			name:   "drops out-of-scope SANs",
			body:   `[{"name_value":"a.example.com\nattacker.evil.com","common_name":"a.example.com"}]`,
			target: "example.com",
			want:   []string{"a.example.com"},
		},
		{
			name:   "lowercases names",
			body:   `[{"name_value":"API.Example.COM","common_name":""}]`,
			target: "example.com",
			want:   []string{"api.example.com"},
		},
		{
			name:   "empty response yields nothing",
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
			got := parseCrtSh([]byte(tt.body), tt.target)
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

func TestCrtShMergesAcrossTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond per-domain; the wildcard query is q=%.<domain>.
		switch r.URL.Query().Get("q") {
		case "%.example.com":
			w.Write([]byte(`[{"name_value":"a.example.com","common_name":""}]`))
		case "%.example.org":
			w.Write([]byte(`[{"name_value":"b.example.org","common_name":""}]`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	restore := stubHTTPGet(srv.URL)
	defer restore()

	got, err := CrtSh(context.Background(), []string{"example.com", "example.org"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"a.example.com", "b.example.org"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCrtShFailsSoftOnPartialError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "%.dead.com" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`[{"name_value":"a.example.com","common_name":""}]`))
	}))
	defer srv.Close()
	restore := stubHTTPGet(srv.URL)
	defer restore()

	got, err := CrtSh(context.Background(), []string{"dead.com", "example.com"})
	if err != nil {
		t.Fatalf("a dead target should not fail the run: %v", err)
	}
	if len(got) != 1 || got[0] != "a.example.com" {
		t.Fatalf("got %v, want [a.example.com]", got)
	}
}

func TestCrtShErrorsWhenAllTargetsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	restore := stubHTTPGet(srv.URL)
	defer restore()

	if _, err := CrtSh(context.Background(), []string{"example.com"}); err == nil {
		t.Fatal("want error when every lookup fails, got nil")
	}
}

// stubHTTPGet routes the package httpGet at base (ignoring the real crt.sh
// host) while preserving the query string, then returns a restore func.
func stubHTTPGet(base string) func() {
	orig := httpGet
	httpGet = func(ctx context.Context, rawURL string) ([]byte, error) {
		i := indexQuery(rawURL)
		if i < 0 {
			return nil, errors.New("no query")
		}
		return orig(ctx, base+"/"+rawURL[i:])
	}
	return func() { httpGet = orig }
}

func indexQuery(u string) int {
	for i := 0; i < len(u); i++ {
		if u[i] == '?' {
			return i
		}
	}
	return -1
}
