# User-service architecture

## Context

The former service architecture created a second Surge identity. An elevated
daemon resolved root/SYSTEM configuration, state, runtime files, and tokens,
while the CLI and TUI resolved the interactive user's files. That split caused
two related failure modes:

- [Issue 645](https://github.com/SurgeDM/Surge/issues/645): category paths were
  configured for the user but absent from the daemon's root-owned settings, so
  service downloads did not use the expected categories.
- [Issue 671](https://github.com/SurgeDM/Surge/issues/671): a connected TUI did
  not have an authoritative daemon settings channel and could display defaults
  or save settings to the wrong machine.

The target model is deliberately smaller: a managed daemon runs as the user who
installed it, and every local mode resolves one configuration identity.

```text
user CLI / local TUI / browser extension
                  |
                  v
       user-owned Surge daemon
                  |
        +---------+---------+
        |         |         |
     settings   state     runtime
      (user)    (user)     (user)
```

Remote clients use the authenticated settings API instead of assuming that
their own local settings represent the daemon.

## Commit-by-commit implementation plan

Each step is independently reviewable and keeps the tree buildable.

1. **Prevent remote settings overwrite.** Give the TUI an injected settings
   persistence function. Make connections to older daemons read-only so a
   synthesized settings snapshot can never overwrite the client's local file.
2. **Add daemon settings synchronization.** Define the shared settings-service
   contract, implement authenticated `GET /settings` and `PUT /settings`, apply
   strict validation and normalization, redact the API token, and report when a
   restart is required.
3. **Introduce the user-service library.** Add a small manager interface for
   install, uninstall, start, stop, restart, and status. Implement systemd user
   units first and route both CLI commands and the TUI auto-start toggle through
   the same library.
4. **Switch Linux fully to user scope.** Generate a user unit that starts
   `surge server start --managed-service`, rejects root installation, and
   detects a conflicting legacy machine-wide unit.
5. **Add native platform backends.** Use a macOS LaunchAgent, a Windows
   current-user scheduled task with limited privileges, and Termux runit. Keep
   the command surface identical across platforms and remove the generic
   privileged service wrapper.
6. **Remove the dual identity.** Delete elevated/root config and runtime
   selection, system-token discovery and mirroring, the internal system-service
   flag, test-only system directory overrides, and the obsolete service
   dependency. Preserve one mode-`0600` token in user state.
7. **Document behavior and migration.** Replace system-service and `sudo`
   guidance, document the shared identity and settings API, give explicit
   legacy-service cleanup steps, and record the operational limitations below.

## Invariants

- Service installation is user-scoped and root installation is rejected.
- Local CLI, TUI, and service processes use the same config, state, runtime,
  database, category paths, and token for that OS user.
- The auth token is stored only in user state with mode `0600`; it is not
  mirrored into a world-readable runtime file.
- A remote TUI saves through the daemon settings API. If that API is absent,
  settings are read-only.
- Installation detects a known legacy system service rather than allowing two
  service identities to coexist silently.

## Tradeoffs and risks

### Short term

- Existing system-service users must remove the old unit/service and install the
  new user service. Root's old settings are not migrated automatically because
  ownership, secrets, and path meanings cannot be inferred safely.
- User services generally start at login, so behavior differs from a
  machine-wide boot service. Linux lingering is an administrator-controlled
  opt-in, not something Surge enables.
- Native supervisors report status differently. The shared manager intentionally
  exposes only the small common state Surge needs, which limits diagnostic
  detail in the CLI.
- Updating an installation that changes the executable path can leave a stale
  service definition until `surge service install` is run again.

### Long term

- Multi-user hosts can run multiple independent Surge daemons. Automatic port
  selection avoids a simple bind collision, but users can still point downloads
  at the same destination and race at the filesystem level.
- Login-session services inherit platform lifecycle constraints: logout policy,
  sleep, encrypted-home availability, network mounts, and Android background
  limits.
- A user service is intentionally not a replacement for an administrator-owned,
  pre-login server. Always-on deployments should use a container or a future
  explicitly designed system deployment with a dedicated unprivileged account
  and an explicit config path—not implicit root behavior.
- The settings HTTP contract becomes compatibility surface. New settings need
  validation, secret-redaction review, and restart-semantics tests.

## Verification

- Unit tests cover settings validation, remote persistence, user-service state,
  token location and permissions, and exclusion of system candidates.
- The complete Go test suite must pass.
- Linux tests run natively; macOS, Windows, and Android/Termux builds are
  cross-compiled to enforce build-tag and interface compatibility.
- Documentation is searched for removed commands (`surge service token`) and
  obsolete elevation guidance before release.
