# `surge service`

Manage Surge as a service owned by the current user. It runs the headless server
with the same settings, state, category paths, and API token as that user's CLI
and TUI. Service management must be run without `sudo` or elevation.

## Commands

```bash
surge service install    # install and start
surge service start
surge service stop
surge service restart
surge service status
surge service uninstall
```

`install` registers the native user service and starts it immediately.
`uninstall` stops it and removes only its service definition; it does not remove
downloads, settings, or state.

Use the ordinary token command for remote TUI or browser-extension setup:

```bash
surge token
```

There is no separate service token or service configuration. This single-user
identity prevents the daemon from falling back to root defaults when the user's
category paths and other settings are expected.

## Platform behavior

- Linux installs `~/.config/systemd/user/surge.service` (or the equivalent
  `$XDG_CONFIG_HOME` path).
- macOS installs `~/Library/LaunchAgents/com.surgedm.surge.plist`.
- Windows creates the current-user scheduled task `SurgeDM\Surge` with limited
  privileges.
- Termux installs a runit service and requires `termux-services`.

The service starts with the user's login/session rather than promising a
pre-login machine boot. See [Run Surge as a user service](../../guides/run-as-a-service.md)
for migration steps, platform limitations, and long-term tradeoffs.

For a short foreground batch, prefer:

```bash
surge server --batch urls.txt --exit-when-done
```
