# CLI startup boundaries

The root command has no inherited engine initialization hook. Startup work belongs
to the command that needs it:

| Workflow | Local engine | Local setup |
| --- | --- | --- |
| Help, version, completion generation, shell completion | None | No settings/history initialization or network requests |
| `config help` | None | Help is handled before loading settings |
| Configuration reads/writes | None | Only the requested configuration operation |
| Explicit remote control commands (`--host` or `SURGE_HOST`) | None | Skip local state directories, settings, history, and logging initialization |
| Remote TUI | None | Local presentation settings are still read for the UI |
| Local control/listing commands | None | Existing local authentication/history/logging setup is retained |
| Local TUI or `server start` | Created after the instance lock | Settings, state, logging, startup integrity, lifecycle worker, and transfers |

`initializeLocalEngine` owns engine replacement and initialization. It drains the
previous local runtime, refreshes settings, prepares local storage, and creates
the scheduler with the existing configured concurrency and channel capacity.
Startup integrity checks and resume handling remain in the owning workflows.
`initializeClientState` skips local initialization only for an explicit remote
target. Explicit inputs such as batch files and CA bundles are still read when
requested. Root `--reset-settings` retains its explicit local reset behavior.

Tests exercise both process and command boundaries. Metadata and an explicit
remote submission run against unusable local settings/state paths; only the
submission contacts the test endpoint. In-process tests ensure completion and
remote commands neither create a local pool nor reset an existing service.
A real-server smoke check verifies that local startup still downloads and
finalizes data.

A local paired timing sample on the shared Linux cloud machine used 25 warm
invocations per command and binary, after two warmups, with output redirected:

| Command | Before p50 / p95 | After p50 / p95 |
| --- | --- | --- |
| `--help` | 7.85 / 21.80 ms | 6.86 / 8.80 ms |
| `--version` | 7.30 / 10.05 ms | 6.97 / 12.58 ms |
| `completion bash` | 9.63 / 26.67 ms | 7.71 / 17.38 ms |

These samples include process launch and normal host variation; they are not
performance guarantees or CI thresholds. Help/version already bypassed Cobra's
pre-run hook. The substantive change removes unnecessary scheduler work from
ordinary subcommands and completion, and local setup from explicit remote clients.
