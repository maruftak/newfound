package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/model"
)

func TestCertExpiringChanges(t *testing.T) {
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	infos := []model.CertInfo{
		{Host: "soon.example.com", Expiry: now.AddDate(0, 0, 10)},  // within 30d window
		{Host: "later.example.com", Expiry: now.AddDate(0, 0, 90)}, // outside window
		{Host: "expired.example.com", Expiry: now.AddDate(0, 0, -5)},
		{Host: "edge.example.com", Expiry: now.AddDate(0, 0, 30)}, // exactly at the cutoff
	}

	changes := certExpiringChanges(infos, 30, now)

	got := map[string]string{}
	for _, c := range changes {
		if c.Kind != diff.CertExpiring {
			t.Errorf("%s: kind = %s, want CERT_EXPIRING", c.Host, c.Kind)
		}
		if c.Priority != diff.High {
			t.Errorf("%s: priority = %d, want High", c.Host, c.Priority)
		}
		got[c.Host] = c.Detail
	}

	if _, ok := got["later.example.com"]; ok {
		t.Error("cert outside the window should not alert")
	}
	if _, ok := got["soon.example.com"]; !ok {
		t.Error("cert within the window should alert")
	}
	if _, ok := got["edge.example.com"]; !ok {
		t.Error("cert at the exact cutoff should alert (boundary inclusive)")
	}
	if d := got["expired.example.com"]; d == "" || !strings.Contains(d, "expired") {
		t.Errorf("expired cert detail should say expired, got %q", d)
	}
	if d := got["soon.example.com"]; !strings.Contains(d, "10 day") {
		t.Errorf("soon detail should report days remaining, got %q", d)
	}
}

func TestCertExpiringChangesEmpty(t *testing.T) {
	if got := certExpiringChanges(nil, 30, time.Now()); len(got) != 0 {
		t.Errorf("no certs should yield no changes, got %+v", got)
	}
}
