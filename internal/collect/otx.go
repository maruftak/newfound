package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maruftak/reconsentry/internal/model"
)

// otxURL builds the AlienVault OTX passive-DNS query for a root domain. The
// basic passive_dns endpoint needs no API key.
const otxURL = "https://otx.alienvault.com/api/v1/indicators/domain/%s/passive_dns"

// otxResponse models the subset of the OTX passive-DNS payload we consume: a
// list of records, each carrying the resolved hostname.
type otxResponse struct {
	PassiveDNS []struct {
		Hostname string `json:"hostname"`
	} `json:"passive_dns"`
}

// OTX discovers subdomains from AlienVault OTX passive-DNS records for the
// given root targets. It is a pure HTTP collector — no external CLI — and is
// always safe to run on passive scopes. A failure for one target does not abort
// the others; an error is returned only when every target lookup fails.
func OTX(ctx context.Context, targets []string) ([]string, error) {
	seen := map[string]bool{}
	var hosts []string
	var errs []error
	for _, t := range targets {
		body, err := httpGet(ctx, fmt.Sprintf(otxURL, t))
		if err != nil {
			errs = append(errs, fmt.Errorf("otx %s: %w", t, err))
			continue
		}
		for _, h := range parseOTX(body, t) {
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

// parseOTX extracts in-scope hostnames from an OTX passive-DNS response. Each
// hostname is lowercased, de-duplicated, and dropped if outside the target
// domain (passive DNS can include unrelated co-resolving names).
func parseOTX(b []byte, target string) []string {
	var resp otxResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil
	}
	target = strings.ToLower(model.TrimInvisible(target))
	seen := map[string]bool{}
	var hosts []string
	for _, rec := range resp.PassiveDNS {
		h := strings.ToLower(model.TrimInvisible(rec.Hostname))
		if h == "" || seen[h] || !inScope(h, target) {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	return hosts
}
