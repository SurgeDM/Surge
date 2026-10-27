# Pause, resume, cancellation, and removal

```bash
surge pause JOB_ID --output-version 1
surge resume JOB_ID --output-version 1
surge cancel JOB_ID --output-version 1
surge pause --all
surge resume --all
```

Pause and cancel request a stop while retaining the record, working file, and
resume metadata. Cancel uses the existing pause/checkpoint mechanism: the final
stopped state is `paused`, and `resume` can restart it. Cancel does not create a
second kind of resume state. Repeating pause/cancel on an already stopped or
completed job succeeds without changing the record. Completion wins over a late
pause acknowledgment. Resume while active, pausing, or already completed returns
HTTP 409 (v1 `conflict`). A missing pause/resume/cancel target returns HTTP 404.

A successful receipt says `requested`, **not checkpointed**. Active workers stop
asynchronously. It does not promise a checkpoint survives power loss or a hard
kill. A transport failure or HTTP 5xx gives an `unknown` outcome; query the job
before deciding whether to retry. The CLI never retries a mutation automatically.
With a cancelled command context, remaining requests say `not_requested`.

`--all` captures one live server snapshot. Pause/cancel select only downloading
and queued jobs; resume selects only paused jobs. Selection is sorted and IDs are
deduplicated. Jobs added afterward are unaffected. Prefix resolution uses the
same live server, never local history. Full IDs bypass queue listing. An empty
selection succeeds. A failed snapshot performs no mutations. Failures preserve
all per-job receipts; a mixed result exits 3 with v1, or 1 with human output.

Human output uses stdout for quoted full IDs and receipts, stderr for the final
error. V1 writes exactly one JSON result. It excludes server response bodies,
source URLs, and credentials. An older server's empty HTTP 200 acknowledgment is
accepted as a request receipt. `/cancel` requires a server with this feature;
the CLI does not substitute `/delete` when it is unsupported.

Removal retains its existing destructive behavior for compatibility:
`rm ID` removes the record, resume metadata, and unfinished `.surge` working
file. Completed artifact files remain. `rm ID --purge` also deletes the completed
artifact. `kill` remains a legacy alias for `rm`; it does not mean non-destructive
cancel. Use `cancel` when retaining resumable data matters. Bulk cleanup flags
cannot be combined with an ID or `--purge`.

Stopping a CLI observer does not transfer ownership of daemon jobs. These
commands affect jobs only through their explicit requests; they do not change
the engine's retry budgets or source-compatibility checks. Foreground ownership
and resume diagnostics are separate additions.
