package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/maruftak/reconsentry/internal/model"
)

// crtShURL builds the certificate-transparency query for a root domain. The
// %25 is a URL-encoded `%` wildcard, so the query matches every certificate
// whose subject ends in the domain.
const crtShURL = "https://crt.sh/?q=%%25.%s&output=json"

// crtShEntry models the subset of a crt.sh JSON record we consume. Each record
// may carry multiple names in name_value (newline-separated) plus a
// common_name; both are mined for hostnames.
type crtShEntry struct {
	NameValue  string `json:"name_value"`
	CommonName string `json:"common_name"`
}

// httpGet is the HTTP fetcher used by the passive collectors. It is a package
// variable so tests can point it at an httptest server.
var httpGet = func(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// CrtSh discovers subdomains from crt.sh certificate-transparency logs for the
// given root targets. It is a pure HTTP collector — no external CLI — and is
// always safe to run on passive scopes. A failure for one target does not abort
// the others; an error is returned only when every target lookup fails.
func CrtSh(ctx context.Context, targets []string) ([]string, error) {
	seen := map[string]bool{}
	var hosts []string
	var errs []error
	for _, t := range targets {
		body, err := httpGet(ctx, fmt.Sprintf(crtShURL, t))
		if err != nil {
			errs = append(errs, fmt.Errorf("crt.sh %s: %w", t, err))
			continue
		}
		for _, h := range parseCrtSh(body, t) {
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

// parseCrtSh extracts in-scope hostnames from a crt.sh JSON response. Wildcard
// entries (`*.example.com`) are unwrapped to their base domain, names are
// lowercased and de-duplicated, and anything not within the target domain is
// dropped (certificates routinely carry unrelated SANs).
func parseCrtSh(b []byte, target string) []string {
	var entries []crtShEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil
	}
	target = strings.ToLower(model.TrimInvisible(target))
	seen := map[string]bool{}
	var hosts []string
	add := func(raw string) {
		h := normalizeCertName(raw)
		if h == "" || seen[h] || !inScope(h, target) {
			return
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	for _, e := range entries {
		for _, name := range strings.Split(e.NameValue, "\n") {
			add(name)
		}
		add(e.CommonName)
	}
	return hosts
}

// normalizeCertName lowercases a certificate name and strips a leading wildcard
// label so `*.example.com` collapses to `example.com`.
func normalizeCertName(raw string) string {
	h := strings.ToLower(model.TrimInvisible(raw))
	h = strings.TrimPrefix(h, "*.")
	return h
}

// inScope reports whether host is the target domain or a subdomain of it.
func inScope(host, target string) bool {
	return host == target || strings.HasSuffix(host, "."+target)
}
