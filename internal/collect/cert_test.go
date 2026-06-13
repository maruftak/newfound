package collect

import (
	"context"
	"errors"
	"testing"
	"time"
)

func stubTLSDial(byHost map[string]time.Time, fail map[string]bool) func() {
	orig := tlsDial
	tlsDial = func(ctx context.Context, host string) (time.Time, error) {
		if fail[host] {
			return time.Time{}, errors.New("handshake failed")
		}
		return byHost[host], nil
	}
	return func() { tlsDial = orig }
}

func TestCertExpiryCollectsExpiries(t *testing.T) {
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	defer stubTLSDial(map[string]time.Time{"a.example.com": exp}, nil)()

	got, err := CertExpiry(context.Background(), []string{"a.example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Host != "a.example.com" || !got[0].Expiry.Equal(exp) {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCertExpiryFailsSoftOnPartialError(t *testing.T) {
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	defer stubTLSDial(
		map[string]time.Time{"ok.example.com": exp},
		map[string]bool{"bad.example.com": true},
	)()

	got, err := CertExpiry(context.Background(), []string{"bad.example.com", "ok.example.com"})
	if err != nil {
		t.Fatalf("one failed host should not fail the batch: %v", err)
	}
	if len(got) != 1 || got[0].Host != "ok.example.com" {
		t.Fatalf("got %+v, want only ok.example.com", got)
	}
}

func TestCertExpiryErrorsWhenAllFail(t *testing.T) {
	defer stubTLSDial(nil, map[string]bool{"x.example.com": true})()

	if _, err := CertExpiry(context.Background(), []string{"x.example.com"}); err == nil {
		t.Fatal("want error when every host fails, got nil")
	}
}
