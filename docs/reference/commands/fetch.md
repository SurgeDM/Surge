# Foreground downloads and verification

```sh
surge fetch https://example.org/model.gguf -o ./models --filename model.gguf --sha256 EXPECTED_64_HEX_DIGEST
surge fetch --resume SESSION_ID --json
surge --host https://downloads.example.org fetch https://example.org/model.gguf --json
```

`fetch` creates an embedded download engine without opening an HTTP listener or
requiring a running daemon. Each local invocation owns an isolated session below
Surge's state directory, so it can coexist with a daemon and other fetch sessions.
The final result includes the full session ID. `--resume SESSION_ID` explicitly
reopens that session, validates existing engine checkpoints, and resumes its jobs.
Only one process may own a session. Saved metadata and source credentials remain
inside its private directory; result receipts exclude URLs and headers.

`-o` is a directory, defaulting to the working directory locally. `--filename`
and `--sha256` apply to one input. All inputs validate before starting any work.
Runtime acceptance failures retain per-input receipts and produce a nonzero exit;
accepted jobs still finish. `get` remains an alias for asynchronous `add`.

Existing destinations fail by default. `--on-exists rename` selects an unused name
at submission. Both policies publish without replacing a destination created by
another process while the download runs; a late collision leaves working data
and returns a conflict. Publication uses native atomic no-replace renames on Linux, macOS, and Windows.
Where that operation is unavailable, a hard link provides the same no-overwrite
guarantee. Filesystems supporting neither primitive fail without publishing and
retain the working file. Legacy downloads keep their existing policy.

An expected SHA-256 is checked in a bounded, cancellable post-transfer read before
publication. Verification uses the existing per-job worker, outside progress
callbacks, rendering, and the serialized persistence worker. It intentionally
adds file I/O only when requested. A mismatch is not automatically retried: the
unpublished `.surge` file is retained for inspection and the receipt reports a
conflict. Resume never silently changes the expected digest. No digest is
computed or claimed when none was requested. Completed receipts label verification
`verified` or `not_requested` and include the digest only when computed.

Ctrl+C or `--timeout` pauses only local invocation-owned jobs and waits up to ten
seconds for shutdown and checkpoint persistence. Another interrupt exits the wait
promptly. `checkpoint_confirmed: false` means the final checkpoint could not be
confirmed; recovery uses the last committed state. SIGINT exits 130; SIGTERM exits
143 where supported. A hard kill provides no final checkpoint guarantee.

An explicit remote host creates daemon-owned jobs. Timeout and interruption detach
by default and leave them running. `--on-interrupt pause|cancel` sends requests
only for IDs with confirmed acceptance from this invocation and reports their
acknowledgments in `interrupt_actions`; an observation timeout still detaches.
Lost submission responses remain `unknown` and are never automatically replayed.
Remote fetch negotiates finalized results, destination policies, and requested
integrity support before submitting. Unsupported features fail before transfer.
A remote result path belongs to the daemon, not the client's filesystem.

`--json` and `--output-version 1` write one v1 result, including failure receipts.
Human output prints final artifact receipts and a copyable session resume command.
Receipts include decimal-string byte counts, reused checkpoint bytes, invocation
duration in seconds, and average newly downloaded bytes per second. The average
includes verification and finalization time. A completed job is returned only
after finalization and persistence, followed by an existence/size check.

The direct API rejects interactive approval requests that specify artifact
policies. Immediate submissions retain the complete policy, including batches;
unsupported services fail instead of silently dropping verification.

Digest-only direct API requests also publish without replacing a late-created
destination. After a successful publication, failure to remove the working hard
link is logged separately and does not change the completed outcome. Transfer
startup refuses to write through a working name linked to an existing artifact,
including when recovering an older checkpoint after a crash.

## Progress and terminal behavior

Progress goes to stderr; final receipts go to stdout. `--progress auto` uses a
compact terminal display when stderr is a terminal, sparse plain lines for human
piped output, and no progress for nonterminal JSON output. `bar`, `plain`, and
`none` select an explicit policy. Bar mode degrades to plain when stderr is not a
terminal or `TERM=dumb`; stdout redirection does not decide animation policy.

`--color auto|always|never` controls color independently. Auto respects `NO_COLOR`.
`--ascii` changes decorations/truncation markers, retaining original filenames.
`--quiet` suppresses human success/progress chatter, never requested JSON.

Frames refresh at most five times per second, coalesce unchanged state, and adapt
to stderr's current width/height. Plain output appends only on state transitions
or after five seconds. Unknown size shows bytes without a percentage or ETA;
verification/finalization suppress transfer-rate claims. Aggregate speed counts
active transfers only; unknown-size jobs are counted explicitly. Per-job ETA may
span days and is absent for invalid rates or unknown totals. Connections show
active/effective configured ceiling when the daemon reports the ceiling.

Rendering runs on its own goroutine using one replaceable scalar snapshot per
owned job. Formatting and writes happen after releasing the snapshot mutex.
Shutdown waits at most 100 ms for a blocked renderer; a frozen terminal cannot
block transfer workers, checkpoint persistence, or the final stdout result.
No alternate-screen or cursor-hiding mode is used. TUI and foreground views share
finite-rate/ETA rules; stopped jobs' historical rates are excluded from live totals.

`checkpoint_bytes` is the saved offset before a resume attempt. Exact reuse can differ when the server rejects ranges or validators change, so resumed results with a nonzero checkpoint report `reused_bytes` and `average_bytes_per_second` as null. Fresh downloads report zero reused bytes and an invocation average including probing and verification.
