# Get started with Surge

This guide gets a download running, then helps you choose whether the
interactive TUI, a headless server, or a user service is the right fit.

## Install Surge

Use the package manager or release artifact that suits your platform:

| Platform | Command or source |
| --- | --- |
| Windows | `winget install surge-downloader.surge`, `scoop install surge`, or `choco install surge` |
| macOS / Linux | `brew install SurgeDM/tap/surge` |
| Arch Linux | `yay -S surge` |
| Nix / NixOS | `nix profile install --refresh github:SurgeDM/Surge` |
| Linux / macOS (no package manager) | `curl -fsSL https://surgedm.github.io/install \| sh` |
| Any supported platform | [Download a release](https://github.com/SurgeDM/Surge/releases/latest) |

The install script detects your OS and architecture (including Linux ARM64,
e.g. Alpine/postmarketOS), downloads the matching release asset, verifies its
checksum, installs the binary to `~/.local/bin` (override with
`SURGE_INSTALL_DIR`), and sets up shell completion for zsh/bash/fish.
Run the same command again to update an existing installation; it reports the
installed and target versions before replacing the binary.

Run `surge --version` after installing to confirm that your shell can find the
binary.

### Nix installation and updates

On Linux or macOS, install Surge into your Nix profile:

```bash
nix profile install --refresh github:SurgeDM/Surge
surge --version
```

This downloads the official Surge release for your machine, verifies its checksum,
and makes `surge` available through your Nix profile's `bin` directory. Nix's shell
setup normally adds that directory to PATH; if `surge` cannot be found, open a new
terminal and check that your Nix shell setup is loaded. The flake supports Intel/AMD
and ARM64 machines on Linux and macOS. Enable `nix-command` and `flakes` in your Nix
configuration if they are not already enabled.

Update an existing installation with:

```bash
nix profile upgrade --refresh Surge
```

`Surge` is the profile entry name created by the install command above. If you
installed with a different flake reference, use `nix profile list` to find its name.
The `--refresh` flag makes Nix fetch the current repository information instead of
reusing its cached copy. New releases become available when the automated release
update PR is merged. Ordinary source commits on `main` do not change the installed
release.

To try Surge without adding it to your profile:

```bash
nix run --refresh github:SurgeDM/Surge
```

On NixOS, add `github:SurgeDM/Surge` as a flake input named `surge`. You can add
`inputs.surge.packages.${pkgs.system}.default` to `environment.systemPackages`, or
import `inputs.surge.nixosModules.default` and set `programs.surge.enable = true`.
The module also supports `programs.surge.systemd.enable = true` for the system
service. Update the `surge` input in your system flake's lockfile and rebuild your
system to upgrade.

## Make your first download

Pass a URL to `surge` to open the TUI with that download queued:

```bash
surge https://example.com/archive.zip
```

By default, the file is saved in the directory from which you ran the command.
Use `--output` to choose a destination:

```bash
surge https://example.com/archive.zip --output ~/Downloads
```

On Windows, use a PowerShell path such as `--output "$HOME\\Downloads"`.

## Choose a mode

Use the **TUI** when you want to watch and manage downloads in a terminal:

```bash
surge
```

Use the **headless server** for a machine without an interactive terminal, or
when commands and the browser extension should control a single background
download manager:

```bash
surge server
```

Use a **user service** when the server should start with your login session:

```bash
surge service install
```

`install` starts the service immediately. Run it as your normal user without
`sudo` or an Administrator terminal.

See [TUI, server, and remote modes](concepts/modes.md) for the differences and
[Run Surge as a service](guides/run-as-a-service.md) before installing a
service.

## Next steps

- [Add, inspect, pause, resume, and remove downloads](guides/download-files.md)
- [Learn the TUI controls](guides/use-the-tui.md)
- [Read the CLI reference](reference/cli.md)
