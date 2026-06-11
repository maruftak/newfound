// Package model defines the core data types shared across reconsentry.
package model

import (
	"sort"
	"strings"
)

// Asset is a single observed item on a target's attack surface. For the v0.1
// MVP an asset is a host; endpoint/param assets are layered on later via the
// katana collector.
type Asset struct {
	Target string   `json:"target"`           // root in-scope domain, e.g. example.com
	Host   string   `json:"host"`             // host/subdomain, e.g. api.example.com
	IP     string   `json:"ip,omitempty"`     // resolved address
	Alive  bool     `json:"alive"`            // responded to an HTTP probe
	Status int      `json:"status,omitempty"` // HTTP status code
	Tech   []string `json:"tech,omitempty"`   // detected technologies
	Title  string   `json:"title,omitempty"`  // page title
	Server string   `json:"server,omitempty"` // Server header
}

// TechString joins the tech list for storage and display.
func (a Asset) TechString() string {
	return strings.Join(a.Tech, ", ")
}

// TrimInvisible strips a UTF-8 BOM (U+FEFF) and surrounding whitespace. A BOM
// survives strings.TrimSpace (U+FEFF is not Unicode whitespace), so hosts read
// from editor-saved config files or shell pipes must be cleaned explicitly —
// otherwise a single invisible byte corrupts every subfinder/httpx call.
func TrimInvisible(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\ufeff", ""))
}

// NormalizeTech returns a sorted copy of t for stable comparison and output.
func NormalizeTech(t []string) []string {
	if len(t) == 0 {
		return nil
	}
	out := append([]string(nil), t...)
	sort.Strings(out)
	return out
}
