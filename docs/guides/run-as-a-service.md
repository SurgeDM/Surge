# Run Surge as a user service

Install a user service when Surge should keep running after you close its
terminal and start again with your login session. The service runs the headless
server as you—not as root—and uses the same settings, download database,
category paths, and API token as your CLI and TUI.

## Install and start

Run the command as your normal user:

```bash
surge service install
surge service status
```

`install` also starts the service. Do not use `sudo` or an Administrator
terminal; Surge rejects root-owned installation so it cannot silently create a
second configuration identity.

Surge uses the platform's user-scoped supervisor:

| Platform | Service mechanism | Starts automatically |
| --- | --- | --- |
| Linux | systemd user unit | When the user's systemd session starts |
| macOS | LaunchAgent | When the user logs in |
| Windows | Current-user scheduled task | When the user logs in |
| Termux | termux-services/runit | With the user's Termux service supervisor |

Termux requires `pkg install termux-services` first.

## Control the service

```bash
surge service start
surge service stop
surge service restart
surge service status
surge service uninstall
```

Normal commands and the browser extension control the same daemon:

```bash
surge add https://example.com/archive.zip
surge ls
surge token
```

Changing settings through a connected TUI updates the daemon's settings file.
Some lifecycle settings apply immediately; settings reported as requiring a
restart take effect after `surge service restart`.

## Migrate an older system service

Surge refuses to install the user service while it detects a legacy
machine-wide service. Stop and remove the old service first so two daemons do
not compete for ports, locks, or download state.

On Linux, start with:

```bash
sudo systemctl disable --now surge
```

Then remove the old `surge.service` unit from `/etc/systemd/system`,
`/usr/lib/systemd/system`, or `/lib/systemd/system`, reload systemd if needed,
and run `surge service install` without `sudo`. On macOS remove the old Surge
LaunchDaemon. On Windows remove the old Surge Windows service from an elevated
terminal, then return to a normal terminal for installation.

The new service intentionally does not read root's old configuration. Review
any settings you want to keep and copy their values into your normal user's
configuration rather than copying ownership-sensitive state wholesale.

## Operational tradeoffs

- A user service normally starts at login, not necessarily at machine boot. On
  Linux, an administrator can opt a trusted account into lingering with
  `loginctl enable-linger USER`, but this changes host policy and is not done by
  Surge.
- Logging out can stop the service on platforms that tie user agents to the
  login session. For an always-on, pre-login server, use a container or an
  explicitly administered deployment instead of running Surge as root.
- Each OS user has independent settings and state. Multiple users can run
  Surge, but they may select different API ports and must not target the same
  download files concurrently.
- The installed definition records the current Surge executable path. Reinstall
  the service after moving the binary if your package manager does not preserve
  that path.
- The service inherits user-session constraints. Sleep, mobile background
  limits, network mounts, and encrypted home directories can pause or delay it.

Uninstalling removes only the service definition. It does not intentionally
delete configuration, state, or downloaded files. See
[data locations](../SETTINGS.md#directory-structure) before removing those
manually.
