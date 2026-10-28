# Nix release packaging and updates

The default Nix package installs an official Surge release archive instead of
building the working tree. `nix/release.json` records the version and the SHA-256
hash of each archive for Linux and macOS on x86_64 and ARM64. `package.nix` derives
the fixed download URLs from that version. Evaluation does not look up a mutable
"latest release" URL. Both `surge` and the compatibility alias `Surge` are installed;
installation checks require their version output to match the recorded release.

The flake pins Nixpkgs 26.05 because 26.11 dropped Intel macOS support. Before
upgrading that input, check support for all four platforms. Intel macOS support
will need revisiting when the 26.05 branch stops receiving updates.

## Automatic release updates

1. **Update Nix release** runs after a successful **Core Build and Release** push
   workflow. It also checks hourly and can be started manually. The completion
   trigger is deliberate: releases published using `GITHUB_TOKEN` do not start
   another workflow through the `release` event. The hourly check also recovers
   when a release was published but a later release-workflow step failed.
2. The updater runs trusted code from `main` with read-only permissions. It finds
   the highest published stable `vMAJOR.MINOR.PATCH` release, excluding drafts,
   prereleases, and `ext-v*` browser-extension releases. An incomplete latest
   release fails the update; it does not silently fall back to an older one.
3. It downloads the published checksum file and all four archives. Every archive
   must match its published SHA-256 checksum and its GitHub asset digest when
   available. Changed checksums for an already recorded version are rejected,
   as are version downgrades. All downloads must pass before writing metadata.
4. If the version changed, the candidate metadata is checked by the reusable
   **Nix** workflow on native Linux and macOS runners for both architectures.
   Package checks run the binaries and assert their versions. A separate check
   evaluates the NixOS package and system-service integration.
5. Only after every check passes does a separate job receive write permission.
   It commits just `nix/release.json` to `chore/nix-release-vVERSION` and opens
   a PR. Repeated runs reuse that branch and open PR without duplicate commits.
   It never force-pushes or bypasses branch protection.
6. Merge the update PR to make that release available through the default flake.
   Users install or upgrade with `--refresh` to bypass cached repository metadata.
   If another release appears during verification, the stale update is skipped;
   a subsequent run handles the newer release.

The repository must allow GitHub Actions to create pull requests (Settings →
Actions → General → Workflow permissions → Allow GitHub Actions to create and
approve pull requests). The updater uses the built-in `GITHUB_TOKEN`; no custom
secret is needed. GitHub may require a maintainer to approve workflows on the
bot-created PR. The updater has already run the Nix checks before opening it.

The earlier Go `vendorHash` update approach is unnecessary for this release
package: dependency updates on `main` do not change the downloaded release. Only
a new core release changes the Nix version and archive hashes.

## Update locally

From the repository root, with Node.js 22+ and Nix installed:

```bash
node .github/scripts/nix-release.cjs
nix flake check --all-systems --no-build --no-write-lock-file
nix flake check --no-write-lock-file --print-build-logs
nix eval --impure --json --file nix/check-nixos.nix
```

The updater uses public GitHub APIs. Set `GH_TOKEN` if you need authenticated API
rate limits; it is never forwarded to archive download hosts. It only changes
`nix/release.json` and leaves it untouched if verification fails.

Run the updater's tests without a token or network access:

```bash
node --test .github/scripts/nix-release.test.cjs
```

The **Nix** workflow also runs on pull requests and pushes to `main`. It checks
the package already recorded in the repository; it does not rewrite hashes to
make an otherwise failing build pass.
