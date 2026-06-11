# ReconSentry

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

1. Write a scope file (`scope.yaml`):

```yaml
name: my-program
targets:
  - example.com
exclude:
  - internal.example.com
min_priority: medium          # low | medium | high
notify:
  webhooks:
    - https://hooks.slack.com/services/XXX/YYY/ZZZ
  slack: true
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

Use `--dry-run` to print changes without sending notifications. State lives in a local
SQLite file (`reconsentry.db` by default; override with `--db`).

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

- [ ] `katana` collector for endpoint/param change tracking
- [ ] `--scan-new`: run nuclei against newly-discovered hosts automatically
- [ ] richer alert formatting and per-change-type routing
- [ ] passive-only mode (no active probing)

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). Good first issues are
labeled `good-first-issue`.

## License

MIT — see [LICENSE](LICENSE).

[sf]: https://github.com/projectdiscovery/subfinder
[hx]: https://github.com/projectdiscovery/httpx
