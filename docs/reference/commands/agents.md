# Agent submissions

Discover contracts without loading configuration, scanning downloads, starting an
engine, or contacting a server:

```
surge capabilities --json
surge capabilities --server --json
```

`--server` explicitly negotiates daemon features with a five-second limit. Older
servers fail clearly when a requested guarantee is unavailable.

Both `add` and `fetch` accept `--manifest FILE` (`-` reads stdin). A manifest is
strict JSON, at most 1 MiB and 256 items. Unknown fields, duplicate references,
unsupported versions, and trailing documents are rejected. All items are
validated before any submission by default. `--allow-partial` submits valid
items and returns correlated errors for invalid items. This is validation before
submission, not a transaction across network requests.

```json
{"schema_version":1,"items":[{"client_ref":"model","url":"https://example.com/model.bin","filename":"model.bin","output":"./models","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","on_exists":"fail"}]}
```

Every item requires a unique `client_ref` (1–128 bytes, no controls) and HTTP(S)
`url`. Optional fields are `mirrors`, `output`, `filename`, `sha256`, and
`on_exists` (`fail`, the default, or `rename`). Manifest destination paths apply
to the engine host. Arguments and global destination overrides cannot be mixed
with a manifest. `--batch -` accepts bounded URL lines from stdin; blank lines and
lines beginning with `#` are ignored. No manifest credentials are echoed in
receipts. Credentials in URLs should still be avoided when possible.

For daemon submissions, use `--idempotency-key KEY` with `add` or remote `fetch`.
The printable ASCII namespace is bounded to 128 bytes. Each item derives a key
from that namespace and its stable reference, or its input position for URL
arguments. A manifest may be reordered while preserving references. Reusing a key
with changed download options returns conflict. The authentication identity
scopes keys; rotating its token creates a new scope.

The daemon persists a stable ID before enqueueing. Concurrent identical requests
wait for the owner's receipt and return the same ID. Clients never automatically
retry a POST. Repeating the same request and key after a lost response recovers
the receipt, including after daemon restart. An accepted receipt means accepted,
not downloaded; use `wait`/`fetch` for a finalized artifact. Ambiguous interrupted
reservations fail closed with HTTP 409 and `X-Surge-Download-ID`; inspect that ID
before deciding whether to submit new work. Pending reservations are never
silently evicted. Accepted receipts expire after 30 days. The ledger holds at most
10,000 records and fails explicitly when full. Its private file stores hashes and
IDs, not source URLs, headers, tokens, or raw keys. This guarantee assumes one
daemon owner for a state directory and a filesystem supporting atomic rename.
Idempotency cannot bypass an interactive approval requirement.

Embedded `fetch` retries use `--resume SESSION`; durable submission namespaces are
remote-only. A malformed manifest creates no session or network request.
