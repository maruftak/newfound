# Contributing to ReconSentry

Thanks for your interest! `reconsentry` is young and contributions are very welcome.

## Development

```bash
go build ./...
go test -race ./...
gofmt -l .        # should print nothing
go vet ./...
```

Please run `gofmt` and `go vet` before opening a PR — CI enforces both.

## Project layout

```
cmd/reconsentry        CLI entrypoint
internal/model      shared types (Asset)
internal/config     scope file parsing + validation
internal/collect    external-tool collectors (subfinder, httpx) + parsers
internal/store      SQLite snapshot persistence
internal/diff       pure snapshot-diff + change classification
internal/prioritize change filtering/ranking
internal/notify     webhook / Slack / Discord delivery
internal/runner     pipeline wiring (discover → probe → store → diff → notify)
```

## Adding a collector

Collectors turn targets/hosts into assets. The pipeline injects two functions
(`DiscoverFunc`, `ProbeFunc`) so they're easy to add and test:

1. Add a thin exec wrapper + a **pure parse function** in `internal/collect`.
2. Unit-test the parser with representative tool output (no network).
3. Wire it into the pipeline in `cmd/reconsentry`.

Keep the exec wrapper thin and put all logic in the testable parse function.

## Good first issues

Look for issues labeled `good-first-issue`. Some starter ideas:

- a `katana` collector for endpoint/param tracking
- a passive-only discovery mode
- additional notifier formats (Telegram, generic JSON templates)

## Style

- Small files, small interfaces, wrap errors with context (`fmt.Errorf("...: %w", err)`).
- Prefer table-driven tests.
- No new third-party dependency without a clear reason.
