const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');

const WORKFLOW_PATH = '.github/workflows/nix-check.yml';
const PACKAGE_PATH = 'package.nix';
const HASH_PATTERN = /^sha256-[A-Za-z0-9+/]{43}=$/;
const SHA_PATTERN = /^[a-f0-9]{40}$/;

function validHash(hash) {
  return typeof hash === 'string' && HASH_PATTERN.test(hash) &&
    `sha256-${Buffer.from(hash.slice(7), 'base64').toString('base64')}` === hash;
}

function hashAssignment(content) {
  const matches = [...content.matchAll(/^[\t ]*vendorHash[\t ]*=[\t ]*"([^"\r\n]+)"[\t ]*;/gm)];
  assert.equal(matches.length, 1, 'Expected exactly one literal vendorHash assignment');
  assert(validHash(matches[0][1]), 'vendorHash must be a canonical SHA-256 SRI hash');
  return matches[0];
}

function replaceHash(content, oldHash, newHash) {
  assert(validHash(newHash), 'Invalid replacement hash');
  const match = hashAssignment(content);
  assert.equal(match[1], oldHash, 'The original hash no longer matches');
  const start = match.index + match[0].indexOf('"') + 1;
  return content.slice(0, start) + newHash + content.slice(start + oldHash.length);
}

function validateUpdate(update) {
  assert(update && typeof update === 'object' && !Array.isArray(update), 'Invalid update data');
  assert.deepEqual(Object.keys(update).sort(),
    ['schema', 'prNumber', 'headSha', 'runId', 'runAttempt', 'oldHash', 'newHash'].sort(),
    'Unexpected update fields');
  assert.equal(update.schema, 1, 'Unsupported update schema');
  for (const key of ['prNumber', 'runId', 'runAttempt']) {
    assert(Number.isSafeInteger(update[key]) && update[key] > 0, `Invalid ${key}`);
  }
  assert(typeof update.headSha === 'string' && SHA_PATTERN.test(update.headSha), 'Invalid head SHA');
  assert(validHash(update.oldHash) && validHash(update.newHash), 'Invalid dependency hash');
  assert.notEqual(update.oldHash, update.newHash, 'The hash has not changed');
}

function command(binary, args, capture = false) {
  return execFileSync(binary, args, {
    encoding: 'utf8',
    stdio: capture ? ['ignore', 'pipe', 'inherit'] : 'inherit',
  });
}

function prepareUpdate() {
  const event = JSON.parse(fs.readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'));
  const pr = event.pull_request;
  assert(pr, 'Expected a pull_request event');
  assert.equal(command('git', ['rev-parse', 'HEAD'], true).trim(), pr.head.sha,
    'Checkout must match the PR head, not its merge commit');
  command('git', ['diff', '--exit-code', 'HEAD', '--']);
  const before = fs.readFileSync(PACKAGE_PATH, 'utf8');
  const oldHash = hashAssignment(before)[1];

  // --inputs-from uses this repository's flake.lock to pin nix-update as well.
  console.log('Calculating vendorHash with nix-update from the locked nixpkgs input...');
  command('nix', ['run', '--inputs-from', '.', 'nixpkgs#nix-update', '--',
    'surge', '--flake', '--version=skip', '--no-src', '--override-filename', PACKAGE_PATH]);

  const after = fs.readFileSync(PACKAGE_PATH, 'utf8');
  const newHash = hashAssignment(after)[1];
  assert.equal(after, replaceHash(before, oldHash, newHash),
    'nix-update changed more than the vendorHash value');
  const files = command('git', ['diff', '--name-only', 'HEAD', '-z'], true).split('\0').filter(Boolean);
  assert(files.every(file => file === PACKAGE_PATH), 'nix-update changed another tracked file');

  console.log(`Calculated vendorHash: ${newHash}`);
  console.log('Building Surge with the calculated hash...');
  command('nix', ['build', '.#surge', '--no-link', '--no-write-lock-file', '--print-build-logs']);

  const changed = oldHash !== newHash;
  if (changed) {
    const update = {
      schema: 1,
      prNumber: pr.number,
      headSha: pr.head.sha,
      runId: Number(process.env.GITHUB_RUN_ID),
      runAttempt: Number(process.env.GITHUB_RUN_ATTEMPT),
      oldHash,
      newHash,
    };
    validateUpdate(update);
    fs.mkdirSync(process.env.NIX_UPDATE_DIR, { recursive: true });
    fs.writeFileSync(path.join(process.env.NIX_UPDATE_DIR, 'update.json'), `${JSON.stringify(update)}\n`);
    fs.writeFileSync(path.join(process.env.NIX_UPDATE_DIR, 'package.nix.patch'),
      command('git', ['diff', '--no-ext-diff', '--binary', 'HEAD', '--', PACKAGE_PATH], true));
  }
  fs.appendFileSync(process.env.GITHUB_OUTPUT, `changed=${changed}\n`);
  const message = changed
    ? `The package builds with \`${newHash}\`, but package.nix must be updated. Repository-owned PRs receive an automatic commit; fork authors can download the artifact and run \`git apply package.nix.patch\`.`
    : 'The committed vendorHash is current and the Nix package builds successfully.';
  fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${message}\n`);
}

async function currentPullRequest({ github, context, core }) {
  const trigger = context.payload.workflow_run;
  const { data: run } = await github.rest.actions.getWorkflowRun({
    ...context.repo, run_id: trigger.id,
  });
  // Re-read the run to reject spoofed names, other events, and old rerun artifacts.
  assert.equal(run.path, WORKFLOW_PATH, 'Unexpected source workflow');
  assert.equal(run.event, 'pull_request', 'Unexpected source event');
  assert.equal(run.status, 'completed', 'Source run is not complete');
  assert.equal(run.conclusion, 'failure', 'Expected a stale-hash failure');
  assert.equal(run.run_attempt, trigger.run_attempt, 'A newer run attempt exists');
  const repository = context.payload.repository;
  if (run.head_repository?.id !== repository.id) {
    core.notice('Fork PRs receive a patch; their branches are not modified.');
    return null;
  }
  // workflow_run.pull_requests can be empty, even for a repository-owned PR.
  const prs = await github.paginate(github.rest.pulls.list, {
    ...context.repo, state: 'open', base: 'main',
    head: `${context.repo.owner}:${run.head_branch}`, per_page: 100,
  });
  const matches = prs.filter(pr => pr.state === 'open' &&
    pr.base.repo.id === repository.id && pr.base.ref === 'main' &&
    pr.head.repo?.id === repository.id && pr.head.ref === run.head_branch &&
    pr.head.sha === run.head_sha && pr.head.ref !== pr.base.ref &&
    pr.head.ref !== repository.default_branch);
  if (matches.length !== 1) {
    core.notice('No unique open PR still matches the tested head; skipping the update.');
    return null;
  }
  return { run, pr: matches[0] };
}

async function findUpdate({ github, context, core }) {
  const current = await currentPullRequest({ github, context, core });
  if (!current) return;
  const artifacts = await github.paginate(github.rest.actions.listWorkflowRunArtifacts, {
    ...context.repo, run_id: current.run.id, per_page: 100,
  });
  const matches = artifacts.filter(artifact =>
    artifact.name === `nix-vendor-hash-${current.run.run_attempt}` && !artifact.expired);
  if (matches.length === 0) {
    core.notice('No verified update artifact; the check failed for another reason.');
    return;
  }
  assert.equal(matches.length, 1, 'Ambiguous update artifacts');
  assert(matches[0].size_in_bytes <= 65536, 'Update artifact is unexpectedly large');
  core.setOutput('artifact-id', String(matches[0].id));
}

async function applyUpdate({ github, context, core }) {
  const current = await currentPullRequest({ github, context, core });
  if (!current) return;
  const manifestPath = path.join(process.env.NIX_UPDATE_DIR, 'update.json');
  const stat = fs.lstatSync(manifestPath);
  assert(stat.isFile() && !stat.isSymbolicLink() && stat.size <= 4096, 'Invalid update manifest');
  const update = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  validateUpdate(update);
  const { run, pr } = current;
  assert.equal(update.prNumber, pr.number, 'Artifact belongs to a different PR');
  assert.equal(update.headSha, pr.head.sha, 'Artifact belongs to a different commit');
  assert.equal(update.runId, run.id, 'Artifact belongs to a different run');
  assert.equal(update.runAttempt, run.run_attempt, 'Artifact belongs to a different attempt');

  const { data: file } = await github.rest.repos.getContent({
    ...context.repo, path: PACKAGE_PATH, ref: update.headSha,
  });
  assert.equal(file.type, 'file', 'package.nix must be a regular file');
  assert.equal(file.encoding, 'base64', 'Unexpected file encoding');
  assert(file.size <= 65536, 'package.nix is unexpectedly large');
  const original = Buffer.from(file.content, 'base64');
  const content = original.toString('utf8');
  assert(Buffer.from(content, 'utf8').equals(original), 'package.nix must be valid UTF-8');
  const updated = replaceHash(content, update.oldHash, update.newHash);

  // GitHub atomically rejects this mutation if the branch moved after our check.
  // No PR code, downloaded script, or patch is executed in this privileged job.
  const result = await github.graphql(`
    mutation($input: CreateCommitOnBranchInput!) {
      createCommitOnBranch(input: $input) { commit { oid url } }
    }
  `, {
    input: {
      branch: {
        repositoryNameWithOwner: context.payload.repository.full_name,
        branchName: pr.head.ref,
      },
      expectedHeadOid: update.headSha,
      message: { headline: 'chore(nix): update Go vendor hash' },
      fileChanges: {
        additions: [{ path: PACKAGE_PATH, contents: Buffer.from(updated).toString('base64') }],
      },
    },
  });
  await core.summary.addRaw(`Updated vendorHash in PR #${pr.number}: ${result.createCommitOnBranch.commit.url}\n\nGitHub may require a maintainer to approve workflows for this bot commit. The Nix check must pass on the new commit before merging.`).write();
}

module.exports = { validHash, hashAssignment, replaceHash, validateUpdate, prepareUpdate, findUpdate, applyUpdate };

if (require.main === module) {
  try {
    prepareUpdate();
  } catch (error) {
    console.error(error);
    process.exitCode = 1;
  }
}
