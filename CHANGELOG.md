# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Telegram notifier: add `notify.telegram` entries (`token` + `chat_id`) to
  alert via the Telegram Bot API. Delivery shares the same retry/backoff as the
  webhook notifiers.
- `run --keep N` prunes old snapshots, retaining only the most recent N per
  scope, so the database stays bounded over months of continuous monitoring
  (default 0 = keep everything).
- Multi-scope configs: a file may declare several scopes under a top-level
  `scopes:` list, and `run` monitors them all in one process (each isolated in
  the database). `assets` / `history` take `--scope <name>` to select one.
  Single-scope files keep working unchanged.
- `reconsentry assets --config scope.yaml [--json]` prints the latest captured
  snapshot (host, liveness, status, IP, tech) straight from the database — no
  re-probing — so the recorded surface is queryable, not a black box.
- `reconsentry history --config scope.yaml [--json]` lists past runs (timestamp
  and asset count, most recent first), making the monitoring cadence and how the
  surface size moved over time visible.
- `reconsentry init [path]` scaffolds a commented starter scope file.
- `--json` flag emits machine-readable run results (one object per cycle) for
  piping into other tooling.
- `--timeout` flag bounds each run cycle so a hung `subfinder`/`httpx` can't
  wedge a continuous monitor (default 10m).
- Webhook delivery now retries transient failures (network errors, HTTP 429/5xx)
  with backoff; permanent 4xx responses fail fast.
- Alert messages include a clickable `https://<host>` URL per change.
- `Makefile`, `.golangci.yml`, `CHANGELOG.md`, issue/PR templates, and a
  golangci-lint CI job.

### Changed
- **Breaking (config):** `notify.slack` and `notify.discord` are now lists of
  webhook URLs instead of booleans. A single scope can now fan out to a generic
  endpoint, Slack, and Discord simultaneously. Update existing scope files:

  ```yaml
  # before
  notify:
    webhooks: [https://hooks.slack.com/services/XXX]
    slack: true
  # after
  notify:
    slack: [https://hooks.slack.com/services/XXX]
  ```
- Notify endpoint URLs are validated (`http://`/`https://`) at config load.

### Fixed
- Strip a UTF-8 BOM (`U+FEFF`) from config files and host values. A BOM
  survives `strings.TrimSpace`, so a scope file saved by a Windows editor
  (e.g. Notepad) silently corrupted every target — the BOM rode along into
  the Host header and made the upstream return the wrong response. Found by
  running the tool against a live target.

### Removed
- Dead `prioritize.Sort` helper (results are already ordered by `diff.Diff`).

## [0.1.0]

### Added
- Initial MVP: `subfinder`/`httpx` collectors, SQLite snapshot store, pure
  snapshot diff with change classification (`NEW_HOST`, `HOST_LIVE`,
  `STATUS_CHANGE`, `IP_CHANGE`, `NEW_TECH`, `HOST_GONE`), priority filtering,
  and webhook/Slack/Discord notifications. Single run or continuous `--interval`
  monitoring. CI, Dockerfile, and GoReleaser config.
