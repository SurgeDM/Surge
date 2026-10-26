# Nix dependency hash updates

Surge commits the Go dependency hash in `package.nix`. Dependency or source
changes can change the generated vendor tree, so PRs must keep this value in
sync. This automation uses `nix-update` to calculate the hash and a separate
workflow to commit a verified correction.

## Flow

1. **Nix / Nix package** runs for every PR targeting `main`, including forks and
   Dependabot. It checks out the exact PR head with a read-only token and no
   persisted Git credentials.
2. It runs `nix-update` with `--flake --version=skip --no-src` and an explicit
   `package.nix` filename. `--inputs-from .` selects `nix-update` from the
   repository's locked nixpkgs input. Neither a release version nor the lockfile
   is updated.
3. It rejects any tracked change beyond the literal `vendorHash` value, then
   builds the resulting Surge package, including its installation checks.
4. If the hash is already correct, the check passes. Otherwise it uploads a
   small manifest and `package.nix.patch`, then fails. A build failure produces
   no update artifact. The check must not pass while the committed hash is stale.
5. **Update Nix vendor hash** runs after that failure. It uses code from the
   default branch and the built-in `GITHUB_TOKEN`; it never checks out or
   executes PR code. It verifies the source workflow, run attempt, open PR,
   repository ownership, tested head SHA, and manifest before replacing just
   the hash in the original `package.nix` fetched from that SHA.
6. A single `createCommitOnBranch` mutation commits the correction to a
   repository-owned PR branch. `expectedHeadOid` makes this atomic: if another
   commit or force-push changes the branch, the mutation fails without modifying
   that branch. The next run handles the new head. A repeated run cannot keep
   committing the same update.

The publisher finds the PR through the API using the source branch and SHA;
it does not trust a PR number supplied by an artifact. GitHub can return an empty
`workflow_run.pull_requests` array even for repository-owned PRs.

## Permissions and CI after the bot commit

No custom secret or GitHub App is required. The calculation workflow has only
`contents: read`. The publisher has `actions: read`, `pull-requests: read`, and
`contents: write`. Dependabot's initial restricted token cannot write; the
separate `workflow_run` workflow receives its own permissions.

GitHub documents that PRs updated with `GITHUB_TOKEN` trigger subsequent PR
workflows in an approval-required state. After an automatic hash commit, a
maintainer may need to select **Approve workflows to run**. Require **Nix package**
on the latest commit before merging; the first stale-hash failure is not a
successful check of the bot's new commit. A GitHub App token can remove that
approval step in a future change if fully unattended operation is needed.

Fork branches are not modified. Their authors download the artifact from the
failed check and apply `package.nix.patch`, or run the update command below.

The publisher workflow and its scripts must reach the default branch before
GitHub can trigger `workflow_run`. This PR can exercise the read-only check;
publishing requires a subsequent PR after merge. Repository rules must also
permit GitHub Actions to commit to the PR branch; protected branches are not
bypassed.

## Update locally

From the repository root, with Nix installed:

```bash
nix --extra-experimental-features 'nix-command flakes' \
  run --inputs-from . nixpkgs#nix-update -- \
  surge --flake --version=skip --no-src --override-filename package.nix

nix --extra-experimental-features 'nix-command flakes' \
  build .#surge --no-link --no-write-lock-file --print-build-logs
```

Tests for the artifact validation, branch checks, and commit payload need only
Node.js and do not contact GitHub:

```bash
node --test .github/scripts/nix-vendor-hash.test.cjs
```

## References

- [nix-update usage and supported dependency hashes](https://github.com/Mic92/nix-update)
- [nix-update command options, including --no-src](https://github.com/Mic92/nix-update/blob/main/nix_update/__init__.py)
- [Selecting flake inputs with --inputs-from](https://nix.dev/manual/nix/stable/command-ref/new-cli/nix3-run.html)
- [GitHub workflow_run permissions and artifact handling](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run)
- [GitHub guidance on untrusted checkouts and privileged workflows](https://docs.github.com/en/actions/reference/security/secure-use#mitigating-the-risks-of-untrusted-code-checkout)
- [GitHub behavior when GITHUB_TOKEN triggers another workflow](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow#triggering-a-workflow-from-a-workflow)
- [Atomic commit input, including expectedHeadOid](https://docs.github.com/en/graphql/reference/commits#createcommitonbranchinput)
- [Commit branch and file-change inputs](https://docs.github.com/en/graphql/reference/git#committablebranch)
