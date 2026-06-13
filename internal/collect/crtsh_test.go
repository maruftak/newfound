package collect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestCrtShFetchesFiltersAndDeduplicatesHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "%.example.com" {
			t.Fatalf("unexpected crt.sh query: %q", got)
		}
		if got := r.URL.Query().Get("output"); got != "json" {
			t.Fatalf("unexpected output query: %q", got)
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(`[
			{"common_name":"a.example.com","name_value":"*.example.com\nB.example.com\nbad.other.test"},
			{"name_value":"https://c.example.com/app\nb.example.com\nexample.com"}
		]`))
	}))
	defer srv.Close()

	got, err := crtSh(context.Background(), []string{"example.com"}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.example.com", "example.com", "b.example.com", "c.example.com"}
	for _, host := range want {
		if !slices.Contains(got, host) {
			t.Fatalf("expected %q in crt.sh hosts, got %v", host, got)
		}
	}
	if slices.Contains(got, "bad.other.test") {
		t.Fatalf("out-of-scope host should be filtered out: %v", got)
	}
}

func TestCrtShReturnsMalformedJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	if _, err := crtSh(context.Background(), []string{"example.com"}, srv.URL); err == nil {
		t.Fatal("expected malformed JSON to return an error")
	}
}

func TestCrtShReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := crtSh(context.Background(), []string{"example.com"}, srv.URL); err == nil {
		t.Fatal("expected non-2xx response to return an error")
	}
}
