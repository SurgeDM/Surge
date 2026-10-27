# CLI output contract

The versioned contract is opt-in. This first version is available for `ls`
(and its alias `l`), with or without a download ID:

```bash
surge ls --output-version 1
surge ls --output-version 1 aabbccdd
```

No `--json` flag is necessary. If both flags are supplied, the selected output
version takes precedence. The flag is local to `ls`; other commands have not
adopted this contract yet. Versioned snapshots cannot be combined with `--watch`.

## Compatibility

Existing `surge ls --json` returns its existing array. Existing
`surge ls ID --json` returns its existing detail object. Their field names,
units, indentation, and empty-list output are unchanged. The legacy exit
convention (0 for success, 1 for error) is retained; rate semantics and freshness
warnings are clarified below.

A successful live response is authoritative, even when empty. Local history is
used only when no local server is discovered, or the local server has a transport
failure or classified temporary unavailability. Authentication, decoding, and
unclassified server errors remain failures. Explicit remote targets never fall
back to local data. These rules also apply to ID-prefix resolution.

Successful v1 snapshots include additive `source` (`live` or `history`) and
`stale` fields. `stale: true` means saved history cannot establish current transfer
state; it does not mean the artifact is corrupt. Clients must tolerate absent
freshness fields on older v1 builds. A snapshot is not proof of artifact integrity.
Legacy JSON retains its array/detail shape; its saved live rate is cleared, with
historical averages retained in the detail object's `avg_speed`. Human output
and legacy JSON commands warn on stderr when displaying saved entries or when
falling back from an unavailable local server. V1 communicates this through
metadata, without a duplicate warning.

## Result envelope

The selected version writes exactly one JSON document to stdout for success or
an operation failure. A detail result contains one item; an empty result and a
failure contain `items: []`, never `null`. There is no ANSI styling or human
preamble in this output. Operation failures do not print a second diagnostic.

```json
{
  "schema_version": 1,
  "operation": "ls",
  "ok": true,
  "items": [
    {
      "id": "aabbccdd-1234-5678-90ab-cdef12345678",
      "filename": "model.gguf",
      "status": "downloading",
      "downloaded_bytes": "1073741824",
      "total_bytes": "4294967296",
      "speed_bytes_per_second": 12582912
    }
  ],
  "source": "live",
  "stale": false
}
```

- `operation` is the canonical command name (`ls` even when invoked as `l`).
- IDs and filenames are not truncated.
- Byte counts are decimal strings representing the backend's signed 64-bit
  integers, avoiding precision loss in JavaScript and other JSON clients.
- `total_bytes` is `null` when the backend does not report a positive total.
  The current backend cannot reliably distinguish an unknown total from an
  empty file, so both are represented as `null`.
- `speed_bytes_per_second` is a number for a downloading job with a finite,
  nonnegative live sample, including zero; it is otherwise `null`. History-backed
  rates are always `null`, and historical averages are not shown as current rates.
- Status is the backend's state string. Clients must tolerate new state strings
  and additional object properties. Field removal, changed units, or changed
  meanings require a new output version.
- Source URLs, headers, and raw transfer errors are deliberately excluded: they
  may contain credentials or signed query parameters. This initial DTO has a
  small stable surface; richer artifact and transfer diagnostics are future work.

The [JSON Schema](schemas/ls-output-v1.schema.json) describes both result shapes.

## Failures and exit status

```json
{
  "schema_version": 1,
  "operation": "ls",
  "ok": false,
  "items": [],
  "error": {
    "code": "not_found",
    "message": "Download not found.",
    "retryable": false
  }
}
```

Use `code` and the exit status for control flow. Messages are descriptive text,
not a parsing interface. `retryable` indicates a classified transient dependency
failure; it is not permission to repeat arbitrary mutations or bypass cooldowns.
Unclassified diagnostics become `operation_failed` with a generic public message;
raw errors and server response bodies are not copied into the JSON result.

| Exit | Code | Meaning |
| --- | --- | --- |
| 0 | — | Snapshot successfully obtained and written |
| 1 | `operation_failed` | Unclassified operation, encoding, or output failure |
| 2 | `invalid_arguments` | Invalid arguments or incompatible flags |
| 3 | `partial_failure` | Reserved for commands adopting per-item batch outcomes |
| 4 | `not_found` | Download or requested server resource not found |
| 5 | `unauthorized` | Server returned HTTP 401 or 403 |
| 6 | `unavailable` | Network connection/DNS failure or HTTP 429, 502, or 503 |
| 7 | `conflict` | Ambiguous ID prefix or HTTP 409 |
| 8 | `timeout` | Context deadline exceeded or HTTP 408/504 |

A successful snapshot can include a job whose state is `failed`: `ok` describes
the listing operation, not the outcome of every listed download. No new signal,
transfer-ownership, or cancellation behavior is introduced by this contract.

Output negotiation follows normal Cobra flag parsing. Put `--output-version 1`
before potentially invalid flags when structured usage errors are required.
If parsing fails before the version is selected, Surge retains the legacy stderr
error and exit 1. Unsupported or empty explicit versions are rejected with a
stderr diagnostic and exit 2; Surge does not silently select another version.
Help remains human-readable even when the version flag is present.

A failed/short stdout write exits unsuccessfully. If a write has already started,
Surge does not attempt a second JSON document; it reports the output failure on
stderr when possible. OS-level broken-pipe signal behavior still applies.

## Implementation guidance

`internal/clioutput` provides typed public failures, exit-code mapping, generic
result envelopes, and writers. It has no Cobra, TUI, engine, or terminal dependency.
Encoding finishes before the first write; write failures remain distinguishable
from operation failures. Public messages must be safe, deliberate strings.

`cmd.Run` is the process boundary used by `main`: it supplies output streams,
executes the selected command, reports an error once, and returns an exit status.
It does not call `os.Exit`. Commands should return errors instead of reporting
and returning the same failure. Commands using the new layer should write via
Cobra's configured writer instead of the process-global stdout.

`ls` separates data lookup from formatting; legacy renderers retain their shapes,
while the versioned DTO is independent of `types.DownloadStatus`. Tests cover
these boundaries, both output modes, error classification, and failed writers.
Future commands must opt into the contract deliberately rather than inheriting
new exit codes or JSON shapes accidentally.
