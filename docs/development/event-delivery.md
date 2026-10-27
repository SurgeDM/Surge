# Event delivery and slow observers

Engine ingress and persistence remain separate from terminal and network output.
`EventBus.InputCh` retains its bounded engine ingress. Stop engine producers
before shutting down the bus; scheduler shutdown already follows that order.

The internal persistence worker uses `SubscribeLifecycle`, via the local service's
`StreamLifecycleEvents`. It receives ordered lifecycle events, skips transient
progress/log/request traffic, and drains accepted events on shutdown. Persistence
may backpressure on storage work. It must not use the lossy observer subscription.
The command's lifecycle cleanup also waits for the worker to finish persistence.

Every observer owns a bounded notification queue, one replaceable progress frame,
and its own output goroutine. The normal progress producer emits complete active
job batches; newer batches replace pending older batches. Individual progress
ticks are also best effort. A lifecycle transition discards pending stale progress.
No observer send waits on terminal or network output inside the dispatcher.
If lifecycle notifications overflow, that observer is disconnected rather than
silently losing a terminal transition. Other observers and persistence continue.

Local service streams resubscribe after overflow and send a system event with
`resync_required: true`. Remote service reconnect status events carry the same
flag only after reconnection and are cancellation-aware. The TUI queries
authoritative state, reconciles
removed/completed jobs, and rejects a snapshot if a lifecycle transition occurred
while it loaded. Transient snapshot failures keep recovery pending with exponential
backoff from one to thirty seconds, logging once per outage. Authentication and
protocol failures do not retry automatically. A successful snapshot also refreshes
the cached aggregate speed. This is recovery from event loss, not a durable
replay protocol.
Transient logs and approval prompts cannot be reconstructed from job history;
the recovery notice tells users that pending approval requests may need resubmission.
Other SSE consumers should refresh `/list` after reconnect or a resync requirement.

SSE batches have a renewed ten-second write deadline where the response writer
supports deadlines. Successful flushes clear the deadline while the stream is
idle, so the next batch does not inherit an expired deadline. Write/flush errors
terminate the handler and clean up its
subscription. Ordinary consoles may block their own writer, but cannot hold the
bus or persistence worker. Progress aggregation skips chunk/rate snapshots while
there are no observers.

Tests cover zero virtual-time observer waits, latest-frame replacement, healthy
subscriber isolation, reliable draining, real file finalization with an unread
observer, concurrent shutdown/cleanup, service recovery, UI reconciliation, and
SSE write failures. `BenchmarkEventBusSaturatedObserver` measures dispatcher cost;
it is not an end-to-end transfer-throughput benchmark.
