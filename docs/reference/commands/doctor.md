# Diagnostics and configuration

```
surge doctor --json
surge doctor --server --json
surge config validate --json
surge config show --json
surge config explain Network.max_concurrent_downloads
```

`doctor` defaults to local, read-only checks of strict configuration parsing,
directory accessibility, saved index format, and up to 256 unfinished working
file paths. It does not open transfer contents, contact sources, acquire ownership,
repair checkpoints, delete orphans, rewrite the index, or create directories.
Working-file presence does not certify a valid resume checkpoint or unchanged
server validators. Foreground session files are checked by `fetch --resume` when
that session is explicitly selected. Missing not-yet-created directories are
informational. Warnings carry recovery guidance; an unreadable index or invalid
configuration fails the command. State indexes larger than 32 MiB require manual
inspection. Diagnostics omit source URLs, tokens, raw errors, and local filenames.

`--server` explicitly runs bounded authenticated capabilities and health requests
with a five-second limit. It checks the selected daemon and does not inspect local
files. A host selection without `--server` is rejected to avoid confusing local
and remote diagnostics.

`config validate`, `show`, and `explain` are local and read-only. They reject corrupt
TOML, unknown fields, invalid types, and invalid values, rather than reporting the
startup loader's recovery defaults as valid configuration. Files are bounded to
1 MiB. Missing settings use defaults. JSON lists value, default, source (`file` or
`default`), description, and whether a restart is required. Authentication tokens
are redacted; proxy URLs expose only scheme and host. These commands reject an
explicit remote host. Existing `config path value`, reset, and editor operations
remain available. Directory write permissions cannot be proven by a read-only
check; a real transfer still reports filesystem failures.

Help groups commands by downloading, managing, and configuration. `surge ui`
explicitly opens the TUI. Bare `surge` in a noninteractive process prints help;
interactive terminals retain the existing TUI entry. `surge completion bash`,
`zsh`, `fish`, and `powershell` use offline metadata and do not start an engine or
contact a daemon. ID arguments deliberately do not query live history during shell
completion. Use `ls` to select identities.
