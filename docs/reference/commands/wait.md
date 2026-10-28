# Wait for daemon downloads

```bash
surge wait FULL_JOB_ID --timeout 10m
surge wait FULL_JOB_ID OTHER_JOB_ID --json --timeout 10m
surge wait FULL_JOB_ID --output-version 1
```

`wait` observes a fixed set of 1–256 IDs. Duplicate resolved IDs are reported
once in first-input order. Prefixes resolve once against one live server snapshot;
ambiguous prefixes fail before observation. Full IDs skip queue listing. No
local-history fallback is used, and no new jobs are submitted or resumed.

The default waits without a time limit. `--timeout` limits observation, including
HTTP reads, and leaves daemon jobs running. Ctrl+C also detaches and returns 130.
An observation timeout returns 8, even when some jobs have completed. Both retain
completed artifact receipts and give errors for unfinished jobs. Human and JSON
output use these exit codes. A paused/cancelled job is a conflict (7); explicitly
resume it first. A removed/missing job returns `not_found` (4). A mixed set of
completed and failed jobs returns `partial_failure` (3). Other classified errors
use the [common exit codes](../cli-output.md#failures-and-exit-status).

`--json` and `--output-version 1` select the same single-document v1 contract.
Without either flag, stdout contains quoted IDs, states, and final paths; errors
use stderr. No cursor control or screen refresh is used. Receipts print at the end
of observation. See the [JSON Schema](../schemas/wait-output-v1.schema.json).

## What completion proves

The daemon's `/download/result?id=...` endpoint uses an optional `ResultService`
interface and declares `X-Surge-Result-Version: 1`. An old or unsupported server
returns a conflict instead of letting the CLI infer completion from transient
progress. Upgrade the daemon and client together to use `wait`.

A completed result requires the lifecycle worker to promote the working file and
commit its completed record. The daemon also checks that the final path names a
regular file of the recorded size when the result is queried. Missing,
inaccessible, or size-changed artifacts return a conflict. Pending finalization
continues to be observed. This is a point-in-time existence/size check, not a
checksum, identity guarantee, or lock preventing later file changes.

A receipt contains an absolute artifact path on the daemon's filesystem,
`size_bytes` as a decimal string, and `verification: "not_requested"`. Byte counts
are strings to preserve 64-bit precision; an unknown total is `null`.

Auto-discovered local daemons produce `location: "local"`. An explicit `--host`
or `SURGE_HOST` produces `location: "remote"` and a `server` address, even for a
loopback address: the client does not assume the server shares its filesystem.
A remote path does not mean a local artifact was downloaded. Source URLs,
credentials, and raw server/transfer error messages are excluded.

## Observation and recovery

Only unfinished IDs are polled. The CLI reuses one HTTP client and performs
sequential targeted GETs, normally one pass per second. It subscribes to no
progress stream, scans no chunk arrays, and writes nothing on the transfer path.
This lets it reconcile completion even when the observer misses every event.

Connection failures, HTTP 429/502/503, and request timeouts use a maximum of six
consecutive attempts per job with delays of 2, 4, 8, 8, and 8 seconds plus request
time. A successful read resets that budget. Overall `--timeout` and Ctrl+C cancel
in-flight reads and backoff immediately. Authentication, protocol, missing-job,
and conflict failures do not retry. This read budget does not alter transfer
retries or mutate daemon jobs. Resuming a failed job is always explicit.

After a terminal result is received it is retained without further polling. The
result describes that observation; an independent client may later remove or
resume a job. `wait` does not provide ownership or transaction isolation across
multiple clients. Artifact digest verification is a separate command capability.
