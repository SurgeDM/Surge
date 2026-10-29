# Checkpoint hashing

`SaveStateWithOptions` persists resume metadata before starting optional payload
hashing. The returned record has an empty `FileHash`; a successful background
job patches only the persisted detail record. Callers must load the detail again
to observe the hash. Existing lifecycle callers using `SkipFileHash` continue to
skip hashing, including the concurrent pause path.

Hashing is best effort. A crash, shutdown, read error, timeout, or saturated
reader limit leaves a valid checkpoint without a hash. Resume already supports
that representation. MD5 remains an accidental-change hint, not an authenticity
check. Existing MD5 and SHA-256 checkpoint formats remain readable.

There are at most two background readers per process, with no waiting queue.
This deliberately sacrifices hash coverage during bursts instead of delaying
saves or accumulating goroutines. `InlineHashTimeout` retains its name for source
compatibility but now supplies the background budget. Cancellation is checked
between filesystem operations; an OS read can outlive the budget. Such a reader
continues occupying its slot, even across backend reconfiguration. `CloseDB`
cancels jobs without waiting for uninterruptible I/O.

Every checkpoint has a unique revision. Publishing requires the same live job,
backend, revision, and detail contents. Save, deletion, URL/status changes,
resume, and backend lifecycle operations invalidate pending work. Hash jobs use
detached records and never mutate caller-owned tasks, headers, or counters.
File identity, size, and modification time must remain stable across hashing.
These checks reject ordinary concurrent writes or replacement; they do not make
external filesystem changes transactional with metadata writes.

Payload reads happen outside `masterMu`. The final detail patch uses the same
lock and atomic write as other store mutations and never rewrites the master
index. Checkpoint encoding and filesystem sync remain synchronous: this change
removes payload hashing latency, not every possible storage delay.
