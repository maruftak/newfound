package collect

import (
	"context"
	"errors"
	"sort"
	"testing"
)

func stubSources(srcs []source) func() {
	orig := defaultSources
	defaultSources = srcs
	return func() { defaultSources = orig }
}

func src(name string, hosts []string, err error) source {
	return source{name: name, fn: func(ctx context.Context, targets []string) ([]string, error) {
		return hosts, err
	}}
}

func TestDiscoverMergesAndDedups(t *testing.T) {
	defer stubSources([]source{
		src("a", []string{"x.example.com", "shared.example.com"}, nil),
		src("b", []string{"shared.example.com", "y.example.com"}, nil),
	})()

	got, err := Discover(context.Background(), []string{"example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"shared.example.com", "x.example.com", "y.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDiscoverFailsSoftWhenOneSourceErrors(t *testing.T) {
	defer stubSources([]source{
		src("dead", nil, errors.New("subfinder not found")),
		src("alive", []string{"x.example.com"}, nil),
	})()

	got, err := Discover(context.Background(), []string{"example.com"})
	if err != nil {
		t.Fatalf("one dead source should not abort discovery: %v", err)
	}
	if len(got) != 1 || got[0] != "x.example.com" {
		t.Fatalf("got %v, want [x.example.com]", got)
	}
}

func TestDiscoverErrorsWhenAllSourcesFail(t *testing.T) {
	defer stubSources([]source{
		src("a", nil, errors.New("boom")),
		src("b", nil, errors.New("bang")),
	})()

	if _, err := Discover(context.Background(), []string{"example.com"}); err == nil {
		t.Fatal("want error when every source fails, got nil")
	}
}
