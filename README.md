# ReconSentry

[![CI](https://github.com/maruftak/reconsentry/actions/workflows/ci.yml/badge.svg)](https://github.com/maruftak/reconsentry/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/maruftak/reconsentry)](https://goreportcard.com/report/github.com/maruftak/reconsentry)
[![Go Reference](https://pkg.go.dev/badge/github.com/maruftak/reconsentry.svg)](https://pkg.go.dev/github.com/maruftak/reconsentry)
[![Release](https://img.shields.io/github/v/release/maruftak/reconsentry?sort=semver)](https://github.com/maruftak/reconsentry/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Know the moment your target's attack surface changes.**

`reconsentry` is a continuous attack-surface change monitor for bug-bounty hunters and
security teams. It watches the targets you're authorized to test and alerts you the
instant a **new subdomain, a newly-live host, a status change, an IP change, or new
technology** appears — so you reach fresh assets before everyone else.

Existing recon tools are great at *discovery* but leave you to diff the output by hand.
`reconsentry` closes that gap: it snapshots your surface on a schedule, computes the
difference, prioritizes what matters, and pushes a clean alert to Slack / Discord / any
webhook.

<!-- TODO: replace with a real terminal demo GIF before launch -->
<!-- ![demo](docs/demo.gif) -->

> ⚠️ **Authorized use only.** Point `reconsentry` at assets you own or domains that are
> explicitly in scope for a bug-bounty / VDP program. Recon against systems you don't
> have permission to test may be illegal.

## How it works

```
targets ──> discover ──> probe ──> snapshot ──> diff vs last run ──> prioritize ──> alert
            (subfinder)  (httpx)   (SQLite)      (NEW_HOST, …)       (low/med/high) (webhook)
```

`reconsentry` orchestrates battle-tested tools instead of reinventing recon. Its value is
the **diff + prioritization + alerting** layer on top.

## Install

`reconsentry` is a single Go binary. It shells out to [`subfinder`][sf] and [`httpx`][hx]
(from ProjectDiscovery) for discovery and probing — install those too.

```bash
# reconsentry
go install github.com/maruftak/reconsentry/cmd/reconsentry@latest

# required recon tools
go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
go install github.com/projectdiscovery/httpx/cmd/httpx@latest
```

> **Note:** ProjectDiscovery's `httpx` can collide on `PATH` with the unrelated Python
> `httpx` CLI. If probing misbehaves, run `httpx -version` and ensure the ProjectDiscovery
> binary resolves first on your `PATH`.

Or build from source:

```bash
git clone https://github.com/maruftak/reconsentry
cd reconsentry
go build -o reconsentry ./cmd/reconsentry
```

## Quick start

1. Scaffold a scope file and edit your targets:

```bash
reconsentry init            # writes a commented scope.yaml
```

```yaml
name: my-program
targets:
  - example.com
exclude:
  - internal.example.com
min_priority: medium          # low | medium | high
# Each list is a set of destination URLs rendered in that platform's format,
# so one scope can alert all three at once.
notify:
  slack:
    - https://hooks.slack.com/services/XXX/YYY/ZZZ
  discord: []
  webhooks: []                # generic JSON POST
```

2. Record a baseline, then monitor:

```bash
# first run records a baseline (no diff)
reconsentry run --config scope.yaml

# run again later — only changes are reported
reconsentry run --config scope.yaml

# or monitor continuously every 6 hours
reconsentry run --config scope.yaml --interval 6h
```

### Flags

| Flag         | Default          | Purpose                                           |
| ------------ | ---------------- | ------------------------------------------------- |
| `--config`   | _(required)_     | path to the scope file                            |
| `--db`       | `reconsentry.db` | SQLite snapshot database                          |
| `--interval` | `0` (run once)   | monitor continuously on this interval (e.g. `6h`) |
| `--timeout`  | `10m`            | max duration per run cycle (`0` = no limit)       |
| `--dry-run`  | `false`          | print changes without sending notifications       |
| `--json`     | `false`          | emit results as JSON (one object per cycle)       |

`--json` makes runs scriptable, e.g. surface only high-priority changes:

```bash
reconsentry run --config scope.yaml --json \
  | jq '.changes[] | select(.priority >= 3) | "\(.kind) \(.host)"'
```

### Inspect the current surface

`run` reports *changes*; `assets` shows the *latest snapshot* straight from the
database, no re-probing — so your recorded surface isn't a black box:

```bash
reconsentry assets --config scope.yaml
# 1 asset(s) for my-program (latest snapshot):
#   app.example.com   live 200  93.184.216.34   [HSTS, Next.js, Vercel]

reconsentry assets --config scope.yaml --json | jq '.[] | select(.alive)'
```

And `history` lists past runs, so you can see the monitoring cadence and how the
surface size moved over time:

```bash
reconsentry history --config scope.yaml
# 2 run(s) for my-program (most recent first):
#   #2     2026-06-11 22:25:16  7 asset(s)
#   #1     2026-06-10 22:25:11  5 asset(s)
```

### Monitor multiple programs

Declare several scopes under a top-level `scopes:` list and `reconsentry run`
monitors them all in one process — each with its own targets, priority, and
notification destinations (see [`examples/multi-scope.yaml`](examples/multi-scope.yaml)):

```yaml
scopes:
  - name: acme-public
    targets: [acme.com]
    notify: { slack: [https://hooks.slack.com/services/XXX] }
  - name: widgets-vdp
    targets: [widgets.example]
    min_priority: high
```

`assets` and `history` then take `--scope <name>` to pick one. Single-scope
files keep working with no changes.

## What it detects

| Change          | Priority | Meaning                                   |
| --------------- | -------- | ----------------------------------------- |
| `NEW_HOST`      | high     | a subdomain that wasn't there before      |
| `HOST_LIVE`     | high     | a known host that just started responding |
| `STATUS_CHANGE` | medium   | HTTP status code changed                  |
| `IP_CHANGE`     | low      | resolved IP changed (opt-in via `track_ip`; off by default — noisy on CDNs) |
| `NEW_TECH`      | low      | a new technology fingerprint              |
| `HOST_GONE`     | low      | a host stopped resolving/responding       |

## Roadmap

- [ ] multiple scopes per config (monitor many programs from one process)
- [ ] `history` / `assets` commands to query the snapshot DB without re-running
- [ ] snapshot retention (`--keep N`) so the DB stays bounded over months
- [ ] Telegram and email notifiers
- [ ] `katana` collector for endpoint/param change tracking
- [ ] `--scan-new`: run nuclei against newly-discovered hosts automatically
- [ ] passive-only mode (no active probing)

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). Good first issues are
labeled `good-first-issue`.

## License

MIT — see [LICENSE](LICENSE).

[sf]: https://github.com/projectdiscovery/subfinder
[hx]: https://github.com/projectdiscovery/httpx
