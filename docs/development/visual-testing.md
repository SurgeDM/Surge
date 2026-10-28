# TUI visual testing

Run the opt-in visual test from a terminal:

```bash
./scripts/visual-test.sh
./scripts/visual-test.sh --theme nord --light
./scripts/visual-test.sh --theme catppuccin --dark
```

The script compiles a temporary Go test binary and runs `TestVisualTUI` directly, preserving terminal input/output. All fixture code lives in `_test.go` files and is excluded from production builds. Normal `go test ./...` skips the interactive test and runs its regression checks.

The test calls `InitialRootModel`, supplies sample download events, chunk progress, and speed-history samples, and uses the normal `RootModel.Update` and `RootModel.View`. Graph colors, progress bars, file details, chunk rendering, and responsive layout have no test-specific implementations or overrides.

Your configured settings, theme, and keymap are used by default. Configuration writes are redirected to a temporary directory; settings edits stay in memory. Download services and OS power actions are stubbed. External application launches are disabled.

The All tab shows queued, downloading, paused, pausing, resuming, completed, and failed downloads. Use a terminal around **160 columns × 56 rows** to see the full dashboard. Smaller sizes exercise the normal responsive layout. Use the normal shortcuts: arrow keys select rows, `0` selects All, `s` opens settings, and `Ctrl+Q` opens quit confirmation.

The production chunk map displays the selected download. Select the paused row to inspect paused chunks; selecting a completed download hides the map, just as in the application. There is no comparison grid or extra legend.

To save a settled ANSI frame without opening the interactive TUI:

```bash
./scripts/visual-test.sh --theme surge --dark --snapshot /tmp/surge-active.ansi
./scripts/visual-test.sh --theme nord --light --select paused --snapshot /tmp/surge-paused.ansi
./scripts/visual-test.sh --width 80 --height 24 --snapshot /tmp/surge-small.ansi
```

`--select` accepts `downloading`, `queued`, `paused`, `pausing`, `resuming`, `completed`, or `error`.
