# Inspect and manage downloads

`surge ls` can inspect the local download database when Surge is not running,
though a running server is needed for live progress. Commands that change the
queue (`pause`, `resume`, `refresh`, `limit`, and `rm`) require a running server.
Use `surge ls` first to find a download ID; an unambiguous ID prefix is accepted
where an ID is required.

## `surge ls`

List all downloads:

```bash
surge ls
```

Show details for one download:

```bash
surge ls ab12
```

Use JSON when a script needs structured output, or watch the queue in a terminal:

```bash
surge ls --json
surge ls --watch
```

`l` is an alias for `ls`.

A successful live list is authoritative, including an empty queue. Saved local
history is used when no server is discovered or the local server is temporarily
unavailable. Authentication, malformed responses, and unclassified server errors
are reported instead of hidden by old records. An explicit `--host` never falls
back to local history. Prefixes resolve against one source, not a merged list of
live and historical IDs. Full IDs query the detail endpoint directly.

Saved entries may have stale status; they do not display historical averages as
current transfer speed. Detail output labels retained averages as `Average`.
Human output and legacy JSON warn on stderr when displaying saved entries or
falling back from an unavailable local server. `ls --output-version 1` instead
includes `source` and `stale` metadata; its history-backed rates are `null`.

Watch checks once per second and writes only changed snapshots. Suitable terminals
use redraws; pipes, non-terminal writers, and empty/`dumb` terminal capabilities
receive append-only plain text without escape sequences. Each watch frame names
its source, and fallback warnings repeat only on a source transition. A refresh
error preserves the last frame before exiting unsuccessfully. No cursor-hiding
or alternate-screen mode is used, and watching does not control transfers.
External filenames and metadata are stripped of terminal controls only for human
presentation; JSON retains original values with JSON escaping.
`--watch` still cannot be combined with JSON output or an individual ID. This is
human watch output, not the future structured event-stream interface.

## `surge pause`

Pause one download:

```bash
surge pause ab12
```

Pause all downloads:

```bash
surge pause --all
```

## `surge resume`

Resume a paused download:

```bash
surge resume ab12
```

Resume every paused download:

```bash
surge resume --all
```

## `surge refresh`

Refresh replaces the source URL of a paused or failed download. This is useful
when a signed URL has expired or the original link no longer works.

```bash
surge refresh ab12 https://example.com/new-signed-url
surge resume ab12
```

The download must be paused or in an error state before it can be refreshed.

## `surge limit`

Set a cap for one download:

```bash
surge limit ab12 2MB/s
```

Set a global cap for the active queue:

```bash
surge limit --global 10MB/s
```

Set the default cap for new downloads:

```bash
surge limit --default 2MB/s
```

Use `0` to make an individual download unlimited. Use `-1` to make it inherit
the default per-download limit.

## `surge rm`

Remove one record from Surge's queue without deleting the downloaded file:

```bash
surge rm ab12
```

Clean up completed or failed records:

```bash
surge rm --clean
surge rm --clean-failed
```

> [!WARNING]
> `surge rm ab12 --purge` also deletes the corresponding file or files from
> disk. `kill` is an alias for `rm`; it has the same behavior.

## Use these commands remotely

Prefix any command with the target server and token:

```bash
surge --host https://192.168.1.10:1700 --token "$SURGE_TOKEN" pause ab12
```

See [remote connections](../../guides/connect-to-a-remote-server.md) for TLS
and token handling.
