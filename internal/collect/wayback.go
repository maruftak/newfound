package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maruftak/reconsentry/internal/model"
)

// waybackURL builds the Web Archive CDX query for a root domain. fl=original
// returns just the archived URLs and collapse=urlkey de-dups them server-side;
// matchType=domain widens `*.domain` to the domain and all its subdomains.
const waybackURL = "http://web.archive.org/cdx/search/cdx?url=*.%s&output=json&fl=original&collapse=urlkey"

// Wayback discovers subdomains from the Internet Archive's Wayback Machine CDX
// index for the given root targets. It is a pure HTTP collector — no external
// CLI — and is always safe to run on passive scopes. A failure for one target
// does not abort the others; an error is returned only when every target lookup
// fails.
func Wayback(ctx context.Context, targets []string) ([]string, error) {
	seen := map[string]bool{}
	var hosts []string
	var errs []error
	for _, t := range targets {
		body, err := httpGet(ctx, fmt.Sprintf(waybackURL, t))
		if err != nil {
			errs = append(errs, fmt.Errorf("wayback %s: %w", t, err))
			continue
		}
		for _, h := range parseWayback(body, t) {
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	// Fail soft: only surface an error if no target produced a result *and* at
	// least one lookup failed, so a single dead query never kills the run.
	if len(hosts) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	return hosts, nil
}

// parseWayback extracts in-scope hostnames from a CDX JSON response. The
// response is an array of single-element rows ([["original"], ["http://..."],
// ...]); the first row is a header and is skipped. Each URL is reduced to its
// host, lowercased, de-duplicated, and dropped if outside the target domain.
func parseWayback(b []byte, target string) []string {
	var rows [][]string
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil
	}
	target = strings.ToLower(model.TrimInvisible(target))
	seen := map[string]bool{}
	var hosts []string
	for i, row := range rows {
		if i == 0 || len(row) == 0 {
			continue // header row, or a malformed empty row
		}
		raw := row[0]
		if i := strings.IndexByte(raw, '?'); i >= 0 {
			raw = raw[:i] // drop query so a path-less URL still parses to a host
		}
		h := cleanHost(raw)
		if h == "" || seen[h] || !inScope(h, target) {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	return hosts
}
