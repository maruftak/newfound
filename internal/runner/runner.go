// Package runner wires the pipeline: discover -> probe -> store -> diff ->
// prioritize -> notify. Collectors are injected so the pipeline is testable
// without the external recon tools.
package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maruftak/newfound/internal/config"
	"github.com/maruftak/newfound/internal/diff"
	"github.com/maruftak/newfound/internal/model"
	"github.com/maruftak/newfound/internal/notify"
	"github.com/maruftak/newfound/internal/prioritize"
	"github.com/maruftak/newfound/internal/store"
)

// DiscoverFunc finds hosts for the given root targets.
type DiscoverFunc func(ctx context.Context, targets []string) ([]string, error)

// ProbeFunc enriches hosts with liveness and metadata.
type ProbeFunc func(ctx context.Context, hosts []string) ([]model.Asset, error)

// Pipeline runs one monitoring cycle.
type Pipeline struct {
	Store     *store.Store
	Discover  DiscoverFunc
	Probe     ProbeFunc
	Notifiers []notify.Notifier
	Now       func() time.Time // injectable clock; defaults to time.Now
}

// Result summarizes one run.
type Result struct {
	RunID      int64
	Assets     []model.Asset
	Changes    []diff.Change
	FirstRun   bool
	NotifyErrs []error
}

// Run executes a single monitoring cycle for cfg.
func (p *Pipeline) Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}

	hosts, err := p.Discover(ctx, cfg.Targets)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	hosts = filterExcluded(hosts, cfg)

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

	res := &Result{RunID: runID, Assets: assets, FirstRun: firstRun}
	if !firstRun {
		changes := diff.Diff(prev, assets)
		res.Changes = prioritize.Filter(changes, prioritize.Level(cfg.MinPriority))
	}

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
