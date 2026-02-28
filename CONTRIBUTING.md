# Contributing

Thanks for checking out Surge. We are very open to contributions and happy to review PRs.

This is intentionally short. If you see something that can be better, open a PR.

## Quick Codebase Map

- `cmd/`: CLI commands and startup wiring (`surge get`, `surge server`, etc.).
- `internal/core/`: service layer (`LocalDownloadService`) that orchestrates add/pause/resume/delete/list.
- `internal/download/`: high-level download flow (`TUIDownload`) and worker-pool lifecycle.
- `internal/engine/`: low-level engine code.
- `internal/engine/probe.go`: probe logic (range support, metadata, mirror probing).
- `internal/engine/concurrent/`: concurrent HTTP downloader and worker/retry/failover logic.
- `internal/engine/single/`: single-connection HTTP downloader fallback.
- `internal/engine/state/`: SQLite-backed persistence for paused/history downloads.
- `internal/tui/`: terminal UI models, update loop, views.
- `internal/testutil/`: mock HTTP servers and test helpers.

If you are looking for networking behavior, start here:

1. `internal/engine/probe.go`
2. `internal/engine/concurrent/`
3. `internal/engine/single/`

## Run Tests

From repo root:

```bash
go test ./...
```

Useful focused runs:

```bash
go test ./internal/engine/concurrent -run TestConcurrentDownloader_SwitchOn429 -count=1
go test ./internal/download -run TestIntegration_PauseResume -count=1
go test ./internal/tui -count=1
```

## PR Expectations

- Keep PRs focused and readable.
- Add or update tests for behavior changes.
- Run `go test ./...` before opening/updating the PR.
- If behavior or CLI usage changes, update docs (`README.md` or `docs/`).

That is it. If you are unsure about approach, open a draft PR early and we can iterate on it together.
