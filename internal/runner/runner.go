// Package runner wires the pipeline: discover -> probe -> store -> diff ->
// prioritize -> notify. Collectors are injected so the pipeline is testable
// without the external recon tools.
package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/model"
	"github.com/maruftak/reconsentry/internal/notify"
	"github.com/maruftak/reconsentry/internal/prioritize"
	"github.com/maruftak/reconsentry/internal/store"
)

// DiscoverFunc finds hosts for the given root targets.
type DiscoverFunc func(ctx context.Context, targets []string) ([]string, error)

// ProbeFunc enriches hosts with liveness and metadata.
type ProbeFunc func(ctx context.Context, hosts []string) ([]model.Asset, error)

// ScanFunc scans newly-found hosts for vulnerabilities/exposures.
type ScanFunc func(ctx context.Context, hosts []string) ([]model.Finding, error)

// CrawlFunc crawls live hosts and returns the endpoints (URLs/params) found.
type CrawlFunc func(ctx context.Context, hosts []string) ([]model.Endpoint, error)

// Pipeline runs one monitoring cycle.
type Pipeline struct {
	Store     *store.Store
	Discover  DiscoverFunc
	Probe     ProbeFunc
	Scanner   ScanFunc  // optional; when set, newly-found hosts are scanned (run --scan-new)
	Crawler   CrawlFunc // optional; when set, live hosts are crawled for endpoint changes (run --crawl)
	Notifiers []notify.Notifier
	Keep      int              // if > 0, retain only the most recent Keep snapshots per scope
	Now       func() time.Time // injectable clock; defaults to time.Now
}

// Result summarizes one run.
type Result struct {
	RunID      int64
	Assets     []model.Asset
	Changes    []diff.Change
	Findings   []model.Finding
	Endpoints  []model.Endpoint
	FirstRun   bool
	NotifyErrs []error
	ScanErr    error
	CrawlErr   error
}

// Run executes a single monitoring cycle for cfg.
func (p *Pipeline) Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}

	discovered, err := p.Discover(ctx, cfg.Targets)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	hosts := filterExcluded(dedupHosts(cfg.Targets, discovered), cfg)

	probed, err := p.Probe(ctx, hosts)
	if err != nil {
		return nil, fmt.Errorf("probe: %w", err)
	}
	assets := reconcile(hosts, probed, cfg.Targets)

	prev, err := p.Store.LatestAssets(cfg.Name)
	if err != nil {
		return nil, err
	}
	firstRun := len(prev) == 0

	runID, err := p.Store.SaveRun(cfg.Name, now(), assets)
	if err != nil {
		return nil, err
	}
	if p.Keep > 0 {
		if err := p.Store.Prune(cfg.Name, p.Keep); err != nil {
			return nil, err
		}
	}

	res := &Result{RunID: runID, Assets: assets, FirstRun: firstRun}

	var changes []diff.Change
	if !firstRun {
		changes = diff.Diff(prev, assets)
		if !cfg.TrackIP {
			changes = dropKind(changes, diff.IPChange)
		}
		if p.Scanner != nil {
			if hosts := newlyFoundHosts(changes); len(hosts) > 0 {
				findings, err := p.Scanner(ctx, hosts)
				if err != nil {
					res.ScanErr = err
				} else {
					res.Findings = findings
					changes = append(changes, findingChanges(findings)...)
				}
			}
		}
	}

	// Crawling runs independently of the host-level firstRun: the first crawl is
	// a baseline (no endpoint diff), later crawls report NEW_ENDPOINT changes.
	if p.Crawler != nil {
		changes = append(changes, p.crawl(ctx, cfg, runID, assets, res)...)
	}

	res.Changes = prioritize.Filter(changes, prioritize.Level(cfg.MinPriority))

	for _, n := range p.Notifiers {
		if err := n.Notify(ctx, cfg.Name, res.Changes); err != nil {
			res.NotifyErrs = append(res.NotifyErrs, err)
		}
	}
	return res, nil
}

func filterExcluded(hosts []string, cfg *config.Config) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if !cfg.Excluded(h) {
			out = append(out, h)
		}
	}
	return out
}

// dropKind removes all changes of the given kind.
func dropKind(changes []diff.Change, k diff.Kind) []diff.Change {
	out := make([]diff.Change, 0, len(changes))
	for _, c := range changes {
		if c.Kind != k {
			out = append(out, c)
		}
	}
	return out
}

// newlyFoundHosts returns the hosts from NEW_HOST / HOST_LIVE changes — the
// hosts worth scanning when --scan-new is set.
func newlyFoundHosts(changes []diff.Change) []string {
	seen := make(map[string]bool)
	var hosts []string
	for _, c := range changes {
		if c.Kind != diff.NewHost && c.Kind != diff.HostLive {
			continue
		}
		if c.Host != "" && !seen[c.Host] {
			seen[c.Host] = true
			hosts = append(hosts, c.Host)
		}
	}
	return hosts
}

// findingChanges turns scanner findings into VULN_FOUND changes so they flow
// through prioritization and notification like any other change.
func findingChanges(findings []model.Finding) []diff.Change {
	out := make([]diff.Change, 0, len(findings))
	for _, f := range findings {
		out = append(out, diff.Change{
			Kind:     diff.VulnFound,
			Host:     f.Host,
			Detail:   fmt.Sprintf("%s: %s (%s)", strings.ToUpper(f.Severity), f.Name, f.TemplateID),
			Priority: severityPriority(f.Severity),
		})
	}
	return out
}

// severityPriority maps a nuclei severity to a change priority.
func severityPriority(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "high":
		return diff.High
	case "medium":
		return diff.Medium
	default: // low, info, unknown
		return diff.Low
	}
}

// dedupHosts merges seed targets with discovered hosts (targets are always
// monitored, even when discovery returns nothing) and removes duplicates.
func dedupHosts(targets, discovered []string) []string {
	all := append(append([]string{}, targets...), discovered...)
	seen := make(map[string]bool, len(all))
	out := make([]string, 0, len(all))
	for _, h := range all {
		h = strings.ToLower(model.TrimInvisible(h))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// reconcile merges discovered hosts with probe results. Probed hosts are
// alive; discovered-but-unprobed hosts are recorded as not-alive so they can
// later transition to HOST_LIVE.
func reconcile(hosts []string, probed []model.Asset, targets []string) []model.Asset {
	byHost := make(map[string]model.Asset, len(probed)+len(hosts))
	for _, a := range probed {
		a.Target = matchTarget(a.Host, targets)
		byHost[a.Host] = a
	}
	for _, h := range hosts {
		if _, ok := byHost[h]; !ok {
			byHost[h] = model.Asset{Host: h, Target: matchTarget(h, targets)}
		}
	}
	out := make([]model.Asset, 0, len(byHost))
	for _, a := range byHost {
		out = append(out, a)
	}
	return out
}

// matchTarget returns the most specific in-scope root that host belongs to.
func matchTarget(host string, targets []string) string {
	best := ""
	for _, t := range targets {
		if host == t || strings.HasSuffix(host, "."+t) {
			if len(t) > len(best) {
				best = t
			}
		}
	}
	return best
}

// liveHosts returns the hostnames of assets that responded, as crawl targets.
func liveHosts(assets []model.Asset) []string {
	var hosts []string
	for _, a := range assets {
		if a.Alive {
			hosts = append(hosts, a.Host)
		}
	}
	return hosts
}

// crawl walks the live hosts, stores the endpoints, and returns NEW_ENDPOINT
// changes against the previous crawl (none on the first crawl). Errors are
// recorded on res rather than failing the whole run.
func (p *Pipeline) crawl(ctx context.Context, cfg *config.Config, runID int64, assets []model.Asset, res *Result) []diff.Change {
	prevEndpoints, err := p.Store.LatestEndpoints(cfg.Name)
	if err != nil {
		res.CrawlErr = err
		return nil
	}
	eps, err := p.Crawler(ctx, liveHosts(assets))
	if err != nil {
		res.CrawlErr = err
		return nil
	}
	for i := range eps {
		eps[i].Target = matchTarget(eps[i].Host, cfg.Targets)
	}
	res.Endpoints = eps
	if err := p.Store.SaveEndpoints(runID, cfg.Name, eps); err != nil {
		res.CrawlErr = err
		return nil
	}
	if len(prevEndpoints) == 0 {
		return nil // first crawl is a baseline
	}
	return diff.DiffEndpoints(prevEndpoints, eps)
}
