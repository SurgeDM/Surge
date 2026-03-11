# Architecture

## Goal

Surge should support multiple user interfaces without changing backend behavior.
The same download system must work behind:

- CLI commands
- Bubble Tea TUI
- Remote clients
- HTTP API clients
- Browser extensions

To make that possible, the codebase should be split into four layers with strict ownership boundaries:

1. UI layer
2. Application / Runtime layer
3. Per-download Processing layer
4. Engine layer

This document describes the target architecture, not the exact current implementation.

## Core Principles

- UI code must not own download logic.
- Global process state must be owned by a UI-agnostic runtime layer.
- Single-download lifecycle logic must be isolated from global application concerns.
- The engine must only execute downloads and report events.
- The engine must be protocol-agnostic at its boundary, even if HTTP is the first implementation.
- The engine must be storage-agnostic and work against injected storage primitives rather than raw `os.File` assumptions.
- Settings, persistence, and HTTP server startup must not leak into the UI or engine.
- Global cross-download policies such as throttling should be injected downward, not implemented by runtime micromanaging workers.
- UIs should consume debounced runtime snapshots rather than raw high-frequency engine events.
- Crash recovery must be a first-class startup phase of the runtime.
- Secrets should be owned by runtime-facing services, not by the engine.
- Path resolution, probing, `.surge` handling, and cleanup must not be spread across multiple layers.

## Layer Overview

### 1. UI Layer

The UI layer is any surface that accepts user intent and renders state.

Examples:

- CLI commands
- TUI screens
- Remote TUI client
- HTTP handlers
- Browser extension bridge

Responsibilities:

- Collect user input
- Send commands to the Application / Runtime layer
- Subscribe to state snapshots and events
- Render output
- Maintain view-only state such as focus, cursor position, modal state, filters, and layout

Must not own:

- DB initialization
- settings loading and persistence
- global queue state
- workerpool lifecycle
- download probing
- filename inference
- directory routing
- `.surge` creation or cleanup
- pause/resume implementation details

The UI layer should be replaceable without changing download behavior.

### 2. Application / Runtime Layer

The Application / Runtime layer is the global backend for a single Surge process.
It is shared by every UI and contains all process-wide, non-UI-specific state.

This is the layer every UI talks to.

Responsibilities:

- Initialize and own the state database
- Load, cache, save, and publish settings
- Own secret and auth configuration services
- Start and stop the HTTP server
- Own the engine instance
- Own the event bus / subscriptions
- Maintain global state snapshots
- Maintain queue-wide and process-wide metrics
- Track active downloads
- Compute aggregate speed and other global stats
- Own global throttling and other process-wide execution policies
- Create shared injected primitives such as global rate limiters
- Debounce, coalesce, and snapshot high-frequency engine events for UI consumers
- Expose list/history/status APIs
- Create and coordinate per-download processing objects
- Reconcile orphaned and interrupted downloads during startup before accepting UI traffic
- Handle application lifecycle and shutdown

Must not own:

- Bubble Tea models or CLI flag UX
- per-download probing and path resolution logic
- low-level HTTP chunk downloading

Important ownership rule:

- The runtime layer owns the engine instance.
- The engine owns the actual download workerpool.
- The runtime layer must not implement a second workerpool for byte downloading.

Good names for this layer:

- `internal/app`
- `internal/runtime`

### 3. Per-download Processing Layer

The Processing layer owns the lifecycle of exactly one download.
It turns a user request into a fully resolved download job.

One processing object should exist per download request / managed download.

Responsibilities:

- Probe the server
- Infer or validate filename
- Decide destination directory
- Resolve conflicts and generate stable final paths
- Create and reserve the `.surge` working file
- Build a protocol-specific execution manifest
- Build the storage backend used by the engine
- Resolve request-specific auth and transport configuration
- Persist per-download lifecycle state through runtime services
- Handle duplicate detection for that download
- Handle pause and resume semantics for that download
- Restore resume metadata
- Verify and recover interrupted `.surge` state when asked by runtime reconciliation
- Finalize completion
- Clean up `.surge` files on cancel or failure
- Emit high-level lifecycle events

This layer is where single-download policy lives.

Must not own:

- process-wide settings storage
- DB bootstrap
- HTTP server bootstrap
- UI rendering
- low-level chunk transport logic

### 4. Engine Layer

The Engine layer executes download jobs.
It should receive a fully resolved job and only do transport and scheduling work.

Responsibilities:

- Own the download workerpool
- Queue jobs for execution
- Schedule workers
- Execute protocol manifests such as HTTP first, and other protocols later
- Manage chunking and retries
- Handle mirrors and failover
- Consume injected storage backends and limiters
- Support pause, resume, and cancel hooks
- Emit raw progress, completion, pause, and error events

Must not own:

- DB initialization
- settings file I/O
- category routing
- path selection
- filename inference
- raw secret persistence
- local-disk-only assumptions
- `.surge` policy beyond using the provided working path
- HTTP server startup
- UI concerns

The engine should not decide what to download or where to save it.
It should only execute the job it is given.

## Dependency Direction

Dependencies should flow downward only:

`UI -> Runtime -> Processing -> Engine`

Events and state updates flow upward:

`Engine -> Processing -> Runtime -> UI`

The lower layer must not import the upper layer.

Global policies flow downward as injected capabilities rather than upward as callbacks:

`Runtime -> Processing -> Engine`

Examples:

- rate limiters
- storage factories
- secret providers
- settings snapshots

## Ownership Matrix

### UI Layer owns

- commands
- screens
- forms
- keybindings
- view models
- rendering

### Runtime Layer owns

- application state
- settings service
- secret and auth service
- DB lifecycle
- engine lifecycle
- HTTP server lifecycle
- subscriptions
- aggregate metrics
- active download registry
- startup reconciliation
- event coalescing and UI snapshot publication
- global rate limiter ownership

### Processing Layer owns

- probe result
- destination resolution
- working file reservation
- protocol manifest construction
- storage backend construction
- request-scoped auth resolution
- per-download cleanup
- pause/resume orchestration for one download
- download-specific persistence writes via runtime services

### Engine Layer owns

- execution queue
- concurrency
- workers
- protocol executors
- limiter consumption
- storage writes through injected interfaces
- chunk scheduler
- HTTP transport
- transport retries

## Data Model Boundaries

Different layers should exchange different kinds of objects.

### UI to Runtime

Use high-level commands such as:

- `EnqueueDownload`
- `PauseDownload`
- `ResumeDownload`
- `DeleteDownload`
- `ListDownloads`
- `GetHistory`
- `UpdateSettings`

These commands should be UI-neutral.

### Runtime to Processing

Use request objects that describe a download intent and provide access to shared services.

Example concerns:

- request id
- URL
- requested output directory
- headers
- mirrors
- settings snapshot
- persistence service
- event publisher
- limiter provider
- storage factory
- auth or transport provider

### Processing to Engine

Use a fully resolved execution job.
By the time the job reaches the engine, there should be no ambiguity left.

Suggested shape:

- stable download id
- protocol manifest
- storage backend
- runtime transport options
- resume metadata or engine checkpoint
- limiter handles
- event sink

The engine boundary should be protocol-agnostic.
Do not hardcode the engine contract around just `URL` plus `filepath`.

Example components:

- `ExecutionJob`: the single object handed from Processing to Engine
- `ProtocolManifest`: protocol-specific description of what to fetch
- `StorageBackend`: random-write capable destination abstraction with finalize semantics
- `EngineCheckpoint`: opaque engine-owned resume payload
- `EngineEvent`: low-level facts emitted upward by the engine

For concurrent downloads, `StorageBackend` should support random writes such as `WriteAt`.
An `io.Writer` alone is not sufficient for ranged, multi-worker downloads.

### Protocol manifests

Processing should decide what protocol is needed and construct the matching manifest.

Examples:

- `HTTPManifest`
- `TorrentManifest`
- `S3Manifest`
- `ExtractorManifest`

This keeps protocol detection and request shaping out of the UI and out of the engine core.

### Storage backends

Processing should hand the engine a storage destination abstraction rather than a raw filesystem path.

Examples:

- local disk with `.surge` promotion on commit
- network filesystem
- object storage multipart writer
- in-memory sink for IPC-style consumers

The engine should write bytes and finalize through the abstraction without caring where the bytes live.

### Throttling

Global throttling should be owned by the Runtime and injected downward.
The engine should consume limiter tokens, not compute global policy itself.

This avoids a split where runtime has policy but engine has no mechanism to enforce it.

### Event flow and backpressure

The engine may emit events at a much higher frequency than any UI can render.

Rules:

- the engine emits raw events
- processing translates them only when needed for lifecycle correctness
- runtime coalesces them into state snapshots
- UIs consume snapshots or debounced event streams

The UI must not attempt to render every low-level engine event.

## Lifecycle of a Download

### Startup and reconciliation

Before accepting UI traffic, the Runtime should reconcile persisted state.

Responsibilities:

- load persisted download records
- identify jobs previously marked as running
- inspect orphaned `.surge` files
- ask Processing to validate and recover per-download state
- transition interrupted jobs into a durable state such as `paused`, `recovered`, or `error`

This makes crash recovery an explicit runtime concern instead of an afterthought.

### Fresh download

1. UI sends an enqueue command to the Runtime layer.
2. Runtime creates a Processing instance for the request.
3. Processing probes the server.
4. Processing resolves filename and destination.
5. Processing reserves the `.surge` file.
6. Processing persists any initial state needed for recovery.
7. Processing submits a resolved job to the Engine.
8. Engine executes the job and emits progress events.
9. Processing reacts to engine events and performs per-download persistence and cleanup.
10. Runtime updates global state and broadcasts snapshots/events to all UIs.
11. UI renders updated state.

### Pause

1. UI requests pause through Runtime.
2. Runtime routes the command to the matching Processing object.
3. Processing asks the Engine to pause the job.
4. Engine stops execution and returns enough state for resume.
5. Processing persists resume metadata and updates lifecycle state.
6. Runtime publishes new global state.

### Resume

1. UI requests resume through Runtime.
2. Runtime routes the command to the matching Processing object.
3. Processing loads persisted resume metadata.
4. Processing rebuilds a fully resolved job.
5. Processing submits that job to the Engine.
6. Runtime republishes state and events.

## Security and Authentication

Authentication material such as headers, cookies, OAuth tokens, and proxy credentials must have clear ownership.

Rules:

- UI collects secrets and sends them to Runtime-facing APIs
- Runtime owns long-lived secret storage policy
- Processing resolves request-scoped auth into protocol-specific execution inputs
- Engine uses prepared auth inputs or prepared clients but does not persist raw credentials

The engine should not become the system of record for secrets.

## Package Direction

Target package layout:

- `cmd/`: thin entrypoints only
- `internal/ui/`: UI implementations
- `internal/app/` or `internal/runtime/`: global runtime layer
- `internal/processing/`: per-download lifecycle logic
- `internal/engine/`: pure execution engine

Suggested package responsibilities:

- `internal/ui/cli`
- `internal/ui/tui`
- `internal/ui/http`
- `internal/runtime`
- `internal/processing`
- `internal/engine`
- `internal/engine/protocols/http`
- `internal/engine/protocols/torrent`
- `internal/storage` or `internal/runtime/storage`
- `internal/state` or `internal/runtime/state` if persistence is considered runtime-owned

## Mapping from Current Repo

The current repository already has pieces of this model, but they are mixed.

### Current code that is already UI-oriented

- `cmd/`
- `internal/tui/`
- browser extension code

### Current code that partially acts like Runtime

- parts of `cmd/root.go`
- parts of `internal/core/`
- global state wiring around service startup and event streams

### Current code that partially acts like Processing

- `internal/processing/`
- parts of `internal/core/` pause/resume orchestration
- parts of `internal/download/` that still contain lifecycle assumptions

### Current code that partially acts like Engine

- `internal/download/worker pool`
- `internal/engine/concurrent/`
- `internal/engine/single/`

The main architectural issue today is that global runtime concerns, per-download lifecycle concerns, and execution concerns are not fully separated.

## Explicit Non-Goals for the Engine

The engine must not:

- open or initialize the DB
- read settings files from disk
- decide category paths
- infer filenames from server responses unless explicitly asked by Processing as a narrow helper
- publish UI-specific events
- know whether the caller is CLI, TUI, or remote
- start network servers
- persist raw credentials
- assume local-disk paths are the only destination model

## Explicit Non-Goals for the UI

The UI must not:

- construct low-level download configs
- guess filenames
- reserve `.surge` files
- decide resume mechanics
- compute aggregate global state from raw worker state
- consume raw high-frequency transport events directly when a runtime snapshot is available

## Refactor Direction

The clean end state is:

- `cmd` becomes thin
- `core` is either removed or reduced to runtime-facing interfaces
- `processing` becomes strictly per-download
- `engine` becomes strictly execution-only
- storage and protocol concerns become explicit interfaces instead of filesystem and HTTP assumptions
- all UIs call the same runtime API

If a new UI can be added without touching download logic, and if a new engine strategy can be added without touching UI code, the architecture is working.
