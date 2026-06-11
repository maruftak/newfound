// Command reconsentry is a continuous attack-surface change monitor: it watches
// authorized targets and alerts when a new subdomain, live host, or technology
// appears.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/maruftak/reconsentry/internal/collect"
	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/diff"
	"github.com/maruftak/reconsentry/internal/notify"
	"github.com/maruftak/reconsentry/internal/runner"
	"github.com/maruftak/reconsentry/internal/store"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.2.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "init":
		os.Exit(cmdInit(os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Printf("reconsentry %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `reconsentry — attack-surface change monitor

usage:
  reconsentry init [path]              write a starter scope file (default scope.yaml)
  reconsentry run --config scope.yaml [flags]
  reconsentry version

run flags:
  --config string   path to scope config (required)
  --db string       path to sqlite database (default "reconsentry.db")
  --interval dur    if set (e.g. 6h), monitor continuously on this interval
  --timeout dur     max duration for a single run cycle (default 10m; 0 = no limit)
  --dry-run         print changes; do not send notifications
  --json            emit results as JSON (one object per cycle) for piping

Only monitor targets you are authorized to test.
`)
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "", "path to scope config (required)")
	dbPath := fs.String("db", "reconsentry.db", "path to sqlite database")
	interval := fs.Duration("interval", 0, "continuous run interval (e.g. 6h); 0 = run once")
	timeout := fs.Duration("timeout", 10*time.Minute, "max duration for a single run cycle (0 = no limit)")
	dryRun := fs.Bool("dry-run", false, "print changes; do not notify")
	jsonOut := fs.Bool("json", false, "emit run results as JSON (one object per cycle)")
	_ = fs.Parse(args)

	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		return 2
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	var notifiers []notify.Notifier
	if !*dryRun {
		add := func(urls []string, format string) {
			for _, u := range urls {
				notifiers = append(notifiers, notify.NewWebhook(u, format))
			}
		}
		add(cfg.Notify.Webhooks, "generic")
		add(cfg.Notify.Slack, "slack")
		add(cfg.Notify.Discord, "discord")
	}

	p := &runner.Pipeline{
		Store:     st,
		Discover:  collect.Subfinder,
		Probe:     collect.Httpx,
		Notifiers: notifiers,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() bool {
		runCtx := ctx
		if *timeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(ctx, *timeout)
			defer cancel()
		}
		res, err := p.Run(runCtx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run error: %v\n", err)
			return false
		}
		if *jsonOut {
			printJSON(cfg.Name, res)
		} else {
			printResult(res)
		}
		return true
	}

	if *interval <= 0 {
		if !runOnce() {
			return 1
		}
		return 0
	}

	if !*jsonOut {
		fmt.Printf("reconsentry: monitoring %q every %s (ctrl-c to stop)\n", cfg.Name, *interval)
	}
	runOnce()
	t := time.NewTicker(*interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if !*jsonOut {
				fmt.Println("reconsentry: stopped")
			}
			return 0
		case <-t.C:
			runOnce()
		}
	}
}

func cmdInit(args []string) int {
	path := "scope.yaml"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		path = args[0]
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "error: %s already exists (refusing to overwrite)\n", path)
		return 1
	}
	if err := os.WriteFile(path, []byte(scopeTemplate), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s — edit your targets, then run: reconsentry run --config %s\n", path, path)
	return 0
}

func printResult(res *runner.Result) {
	for _, e := range res.NotifyErrs {
		fmt.Fprintf(os.Stderr, "notify error: %v\n", e)
	}
	switch {
	case res.FirstRun:
		fmt.Printf("baseline recorded: %d assets (no diff on first run)\n", len(res.Assets))
	case len(res.Changes) == 0:
		fmt.Printf("no changes (%d assets)\n", len(res.Assets))
	default:
		fmt.Printf("%d change(s):\n", len(res.Changes))
		for _, c := range res.Changes {
			fmt.Printf("  [%s] %s — %s\n", c.Kind, c.Host, c.Detail)
		}
	}
}

// runJSON is the machine-readable view of a run, stable for piping into other
// tooling.
type runJSON struct {
	Scope      string        `json:"scope"`
	RunID      int64         `json:"run_id"`
	FirstRun   bool          `json:"first_run"`
	AssetCount int           `json:"asset_count"`
	Changes    []diff.Change `json:"changes"`
}

func printJSON(scope string, res *runner.Result) {
	for _, e := range res.NotifyErrs {
		fmt.Fprintf(os.Stderr, "notify error: %v\n", e)
	}
	out := runJSON{
		Scope:      scope,
		RunID:      res.RunID,
		FirstRun:   res.FirstRun,
		AssetCount: len(res.Assets),
		Changes:    res.Changes,
	}
	if out.Changes == nil {
		out.Changes = []diff.Change{}
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "json error: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

// scopeTemplate is written by `reconsentry init`.
const scopeTemplate = `# reconsentry scope.
# Only monitor targets you own or that are explicitly in scope for an
# authorized bug-bounty / vulnerability-disclosure program.

name: my-program

targets:
  - example.com
  # - example.org

# Hosts/subtrees to ignore (exact host or any subdomain of it).
exclude:
  - internal.example.com

# Minimum priority to report: low | medium | high
min_priority: medium

# Alert when a host's resolved IP changes. Off by default: CDN/cloud hosts
# (Vercel, Cloudflare, ...) rotate IPs constantly and would spam false alerts.
track_ip: false

# Each list is a set of destination URLs rendered in that platform's format.
# A scope can fan out to all three at once.
notify:
  webhooks: []   # generic JSON POST
  slack: []      # Slack incoming-webhook URLs
  discord: []    # Discord webhook URLs
`
