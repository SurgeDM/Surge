# `surge add`

Add one or more URLs to a running Surge server without opening the TUI. `get`
is an alias for this command.

```text
surge add [url]...
surge get [url]...
```

## Before you begin

A local TUI or headless server must already be running. Start one in another
terminal if needed:

```bash
surge
# or, without the TUI
surge server
```

If the TUI was started with `--no-server`, `add` cannot connect to it.

## Add one URL

```bash
surge add https://example.com/releases/surge.tar.gz
```

Surge adds the URL to the running server's queue. Use `surge ls` to find its ID
and monitor the result.

## Save to a specific directory

```bash
surge add https://example.com/releases/surge.tar.gz --output ~/Downloads
```

The output directory defaults to the current working directory. Quote paths
that contain spaces:

```powershell
surge add https://example.com/releases/surge.tar.gz --output "$HOME\My Downloads"
```

## Add several URLs

Pass URLs as separate arguments:

```bash
surge add \
  https://example.com/one.iso \
  https://example.com/two.iso
```

For a longer list, create a plain-text file with one URL per line and use
`--batch`:

```text
# urls.txt
https://example.com/one.iso
https://example.com/two.iso
```

```bash
surge add --batch urls.txt
```

## Review before queuing

Use `--confirm` when a batch file or pasted list needs a final check:

```bash
surge add --batch urls.txt --confirm
```

## Add to a remote server

Supply the remote host and its token before the command:

```bash
surge --host https://192.168.1.10:1700 --token "$SURGE_TOKEN" add \
  https://example.com/releases/surge.tar.gz
```

Remote connections that send a token should use HTTPS. Set `SURGE_HOST` and
`SURGE_TOKEN` when you use the same server regularly. See
[Connect to a remote server](../../guides/connect-to-a-remote-server.md) for
connection and security guidance.

## Related commands

- [`surge ls`](inspect-and-manage.md#surge-ls) shows queued downloads and IDs.
- [`surge pause`, `resume`, and `refresh`](inspect-and-manage.md) control a
  queued download.

## Submission receipts and automation

`add` and `get` report acceptance, not completion. Each human receipt names its
one-based input index and full queued download ID, or a separate approval request
ID. Warnings and errors go to stderr. A mixed batch now exits unsuccessfully
instead of silently treating partial acceptance as success (legacy exit 1).

Use the opt-in v1 contract for a single JSON document and classified exit codes:

```bash
surge add --output-version 1 https://example.com/one.iso https://example.com/two.iso
surge get --output-version 1 --batch urls.txt
```

Each item contains `input_index`, `status`, and, when available, `id` or
`request_id`. Indices follow positional URLs, then URLs parsed from the batch
file; the batch reader's existing comment handling and deduplication still apply.
Source URLs and raw server error bodies are not echoed in receipts.

| Status | Meaning |
| --- | --- |
| `queued` | The server returned a download ID; the transfer is not necessarily started or complete. |
| `pending_approval` | The server accepted an approval request. `request_id` is not a queued job ID or a completion guarantee. |
| `not_submitted` | Local validation prevented submission. |
| `failed` | The server rejected the request; the item includes a safe structured error. |
| `unknown` | A reliable acceptance receipt could not be obtained. The request may have taken effect. Inspect the server before resubmitting. |

With v1, mixed accepted/failed outcomes retain every item and return exit 3 with
`ok: false` and `error.code: partial_failure`. An entirely failed batch uses its
common classified error code, or exit 1 if the errors differ. `items` can also be
empty for an operation-wide failure before submission, such as missing input or
an unavailable local connection. See the [output contract](../cli-output.md).

`--batch FILE --confirm` retains one TUI batch prompt. Pending items share the
batch approval `request_id`; no per-job IDs are invented. Invalid local input
prevents submission of that entire confirmed batch. Without batch confirmation,
inputs are submitted individually and valid inputs can be accepted alongside
invalid ones. No automatic POST retries are performed. Empty, malformed, oversized,
or ID-less success responses and server-side 5xx responses are treated as unknown
outcomes, not permission to repeat a possibly successful mutation.

The [add v1 JSON Schema](../schemas/add-output-v1.schema.json) describes receipts.
