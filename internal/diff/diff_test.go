package diff

import (
	"testing"

	"github.com/maruftak/reconsentry/internal/model"
)

func asset(host string, alive bool, status int, ip string, tech ...string) model.Asset {
	return model.Asset{Target: "example.com", Host: host, Alive: alive, Status: status, IP: ip, Tech: tech}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name string
		prev []model.Asset
		curr []model.Asset
		want []Kind
	}{
		{
			name: "new host",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1")},
			curr: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1"), asset("b.example.com", true, 200, "2.2.2.2")},
			want: []Kind{NewHost},
		},
		{
			name: "host came alive",
			prev: []model.Asset{asset("a.example.com", false, 0, "")},
			curr: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1")},
			want: []Kind{HostLive},
		},
		{
			name: "status change",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1")},
			curr: []model.Asset{asset("a.example.com", true, 403, "1.1.1.1")},
			want: []Kind{StatusChange},
		},
		{
			name: "ip change",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1")},
			curr: []model.Asset{asset("a.example.com", true, 200, "9.9.9.9")},
			want: []Kind{IPChange},
		},
		{
			name: "new tech",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1", "nginx")},
			curr: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1", "nginx", "php")},
			want: []Kind{NewTech},
		},
		{
			name: "host gone",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1")},
			curr: []model.Asset{},
			want: []Kind{HostGone},
		},
		{
			name: "no change",
			prev: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1", "nginx")},
			curr: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1", "nginx")},
			want: nil,
		},
		{
			name: "first-run style empty prev yields all new",
			prev: []model.Asset{},
			curr: []model.Asset{asset("a.example.com", true, 200, "1.1.1.1"), asset("b.example.com", false, 0, "")},
			want: []Kind{NewHost, NewHost},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diff(tt.prev, tt.curr)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d changes %v, want %d %v", len(got), kinds(got), len(tt.want), tt.want)
			}
			for i := range got {
				if got[i].Kind != tt.want[i] {
					t.Errorf("change %d: got %s, want %s", i, got[i].Kind, tt.want[i])
				}
			}
		})
	}
}

func TestDiffOrderingByPriority(t *testing.T) {
	prev := []model.Asset{asset("a.example.com", true, 200, "1.1.1.1", "nginx")}
	curr := []model.Asset{
		asset("a.example.com", true, 200, "1.1.1.1", "nginx", "php"), // NEW_TECH (low)
		asset("z.example.com", true, 200, "2.2.2.2"),                 // NEW_HOST (high)
	}
	got := Diff(prev, curr)
	if len(got) != 2 {
		t.Fatalf("want 2 changes, got %d", len(got))
	}
	if got[0].Kind != NewHost {
		t.Errorf("high-priority change should sort first, got %s", got[0].Kind)
	}
}

func kinds(c []Change) []Kind {
	out := make([]Kind, 0, len(c))
	for _, x := range c {
		out = append(out, x.Kind)
	}
	return out
}
