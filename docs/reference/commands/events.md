# Filtered events

```
surge events FULL-ID [FULL-ID...] > events.ndjson
surge events FULL-ID --progress --interval 1s --timeout 10m
```

The stream is NDJSON version 1 on stdout. It requires 1–256 unique full UUIDs and
uses the authenticated daemon endpoint. It does not submit, pause, or cancel work.
The initial `snapshot` contains sanitized CLI download items. Subscription occurs
before the snapshot is read. Later `state_changed` records invalidate the named
job: retrieve its current status or finalized result before acting. These notices
are deliberately not state patches; a notification buffered before the snapshot
cannot regress the snapshot. `progress` is optional sampled telemetry, not proof
of a terminal outcome. Use `wait` for finalized artifact receipts.

Each record carries `schema_version`, `type`, stream `epoch`, `sequence`, and UTC
`time`. Sequence numbers are monotonic within that connection only. This is not a
durable event journal or replay cursor. `resync_required` means reconnect and
replace cached state from the new snapshot. It closes the command with exit 6.
Unexpected disconnects return a sanitized error record and exit 6. Timeout and
interrupts only detach the observer. Slow output, broken pipes, and disconnects
never cancel daemon jobs. The daemon bounds observer queues and socket write
time, and emits a resync marker when a live subscription overflows.

Lifecycle-only observers do not activate progress aggregation. Optional progress
is limited to one sample per selected job per interval (250ms–1m; default 1s).
Agent streams never request chunk bitmaps and exclude URLs, headers, raw errors,
and approval payloads. Existing TUI SSE streams keep their compatibility behavior.

Errors before stream creation are also one NDJSON `error` record. Stream errors
have no epoch/sequence history; consumers should branch on `type` first. A broken
stdout exits without attempting another write. No automatic reconnect hides gaps.
