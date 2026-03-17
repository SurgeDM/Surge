# CLI Usage

Surge provides a robust Command Line Interface for automation and scripting. For configuration options, see [SETTINGS.md](SETTINGS.md).

## Command Table

| Command                     | What it does                                                                           | Key flags                                                                                           | Notes                                             |
| :-------------------------- | :------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------- | :------------------------------------------------ |
| `surge [url]...`            | Launches local TUI. Queues optional URLs.                                              | `--batch, -b`<br>`--port, -p`<br>`--output, -o`<br>`--no-resume`<br>`--exit-when-done`              | `-o` defaults to CWD. If `--host` is set, this becomes remote TUI mode. |
| `surge server [url]...`     | Launches headless server. Queues optional URLs.                                        | `--batch, -b`<br>`--port, -p`<br>`--output, -o`<br>`--exit-when-done`<br>`--no-resume`<br>`--token` | `-o` defaults to CWD. Primary headless mode command.                    |
| `surge connect [host:port]` | Launches TUI connected to a server. Auto-detects local server when no target is given. | `--insecure-http`                                                                                   | Convenience alias for remote TUI usage.                                 |
| `surge add <url>...`        | Queues downloads via CLI/API.                                                          | `--batch, -b`<br>`--output, -o`                                                                     | `-o` defaults to CWD. Alias: `get`.                                     |
| `surge ls [id]`             | Lists downloads, or shows one download detail.                                         | `--json`<br>`--watch`                                                                               | Alias: `l`.                                                             |
| `surge pause <id>`          | Pauses a download by ID/prefix.                                                        | `--all`                                                                                             |                                                                         |
| `surge resume <id>`         | Resumes a paused download by ID/prefix.                                                | `--all`                                                                                             |                                                   |
| `surge refresh <id> <url>`  | Updates the source URL of a paused or errored download.                                | None                                                                                                | Reconnects using the new link.                    |
| `surge rm <id>`             | Removes a download by ID/prefix.                                                       | `--clean`                                                                                           | Alias: `kill`.                                    |
| `surge token`               | Prints current API auth token.                                                         | None                                                                                                | Useful for remote clients.                        |

## Server Subcommands (Compatibility)

| Command                       | What it does                                           |
| :---------------------------- | :----------------------------------------------------- |
| `surge server start [url]...` | Legacy equivalent of `surge server [url]...`.          |
| `surge server stop`           | Stops a running server process by PID file.            |
| `surge server status`         | Prints running/not-running status from PID/port state. |

## Global Flags

These are persistent flags and can be used with all commands.

| Flag                 | Description                            |
| :------------------- | :------------------------------------- |
| `--host <host:port>` | Target server for TUI and CLI actions. |
| `--token <token>`    | Bearer token used for API requests.    |
| `--verbose, -v`      | Enable verbose logging.                |

## Environment Variables

| Variable      | Description                                   |
| :------------ | :-------------------------------------------- |
| `SURGE_HOST`  | Default host when `--host` is not provided.   |
| `SURGE_TOKEN` | Default token when `--token` is not provided. |

## Fonts

Surge bundles a Nerd Font, but terminal fonts are controlled by your terminal
emulator. Install the bundled font and set your terminal to
`JetBrainsMono Nerd Font Mono`.

See [FONTS.md](FONTS.md) for install steps and licensing details.
