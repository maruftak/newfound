// Command reconsentry is a continuous attack-surface change monitor: it watches
// authorized targets and alerts when a new subdomain, live host, or technology
// appears.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maruftak/reconsentry/internal/collect"
	"github.com/maruftak/reconsentry/internal/config"
	"github.com/maruftak/reconsentry/internal/notify"
	"github.com/maruftak/reconsentry/internal/runner"
	"github.com/maruftak/reconsentry/internal/store"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
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
  reconsentry run --config scope.yaml [flags]
  reconsentry version

run flags:
  --config string   path to scope config (required)
  --db string       path to sqlite database (default "reconsentry.db")
  --interval dur    if set (e.g. 6h), monitor continuously on this interval
  --dry-run         print changes; do not send notifications

Only monitor targets you are authorized to test.
`)
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "", "path to scope config (required)")
	dbPath := fs.String("db", "reconsentry.db", "path to sqlite database")
	interval := fs.Duration("interval", 0, "continuous run interval (e.g. 6h); 0 = run once")
	dryRun := fs.Bool("dry-run", false, "print changes; do not notify")
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
		format := "generic"
		switch {
		case cfg.Notify.Slack:
			format = "slack"
		case cfg.Notify.Discord:
			format = "discord"
		}
		for _, url := range cfg.Notify.Webhooks {
			notifiers = append(notifiers, notify.NewWebhook(url, format))
		}
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
		res, err := p.Run(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run error: %v\n", err)
			return false
		}
		printResult(res)
		return true
	}

	if *interval <= 0 {
		if !runOnce() {
			return 1
		}
		return 0
	}

	fmt.Printf("reconsentry: monitoring %q every %s (ctrl-c to stop)\n", cfg.Name, *interval)
	runOnce()
	t := time.NewTicker(*interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("reconsentry: stopped")
			return 0
		case <-t.C:
			runOnce()
		}
	}
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
