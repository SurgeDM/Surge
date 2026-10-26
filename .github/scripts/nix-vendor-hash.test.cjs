const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const { validHash, hashAssignment, replaceHash, validateUpdate, findUpdate, applyUpdate } = require('./nix-vendor-hash.cjs');

const OLD_HASH = `sha256-${Buffer.alloc(32, 1).toString('base64')}`;
const NEW_HASH = `sha256-${Buffer.alloc(32, 2).toString('base64')}`;
const HEAD = 'a'.repeat(40);
const CONTENT = `# Preserve comments, flags, and formatting.\n{ lib }: {\n  vendorHash = "${OLD_HASH}";\n  version = "1.0.0";\n}\n`;

function fixture(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'surge-nix-test-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const previous = process.env.NIX_UPDATE_DIR;
  process.env.NIX_UPDATE_DIR = directory;
  t.after(() => {
    if (previous === undefined) delete process.env.NIX_UPDATE_DIR;
    else process.env.NIX_UPDATE_DIR = previous;
  });
  const update = {
    schema: 1, prNumber: 12, headSha: HEAD, runId: 34, runAttempt: 1,
    oldHash: OLD_HASH, newHash: NEW_HASH,
  };
  const run = {
    id: 34, path: '.github/workflows/nix-check.yml', event: 'pull_request',
    status: 'completed', conclusion: 'failure', run_attempt: 1,
    head_branch: 'dependency-update', head_sha: HEAD, head_repository: { id: 56 },
    pull_requests: [],
  };
  const pr = {
    number: 12, state: 'open',
    base: { ref: 'main', repo: { id: 56 } },
    head: { ref: 'dependency-update', sha: HEAD, repo: { id: 56 } },
  };
  const file = { type: 'file', encoding: 'base64', size: CONTENT.length, content: Buffer.from(CONTENT).toString('base64') };
  const outputs = {};
  const mutations = [];
  const core = {
    notice() {},
    setOutput(name, value) { outputs[name] = value; },
    summary: { addRaw() { return this; }, async write() {} },
  };
  const github = {
    rest: {
      actions: {
        async getWorkflowRun() { return { data: run }; },
        listWorkflowRunArtifacts() {},
      },
      pulls: { list() {} },
      repos: {
        async getContent(options) {
          assert.equal(options.path, 'package.nix');
          assert.equal(options.ref, HEAD);
          return { data: file };
        },
      },
    },
    async paginate(method) {
      if (method === github.rest.pulls.list) return [pr];
      assert.equal(method, github.rest.actions.listWorkflowRunArtifacts);
      return [{ id: 78, name: 'nix-vendor-hash-1', expired: false, size_in_bytes: 512 }];
    },
    async graphql(query, variables) {
      mutations.push({ query, variables });
      return { createCommitOnBranch: { commit: { oid: 'b'.repeat(40), url: 'https://example.com/commit' } } };
    },
  };
  const context = {
    repo: { owner: 'SurgeDM', repo: 'Surge' },
    payload: {
      repository: { id: 56, full_name: 'SurgeDM/Surge', default_branch: 'main' },
      workflow_run: { id: 34, run_attempt: 1 },
    },
  };
  function writeManifest() {
    fs.writeFileSync(path.join(directory, 'update.json'), JSON.stringify(update));
  }
  writeManifest();
  return { github, context, core, directory, update, run, pr, file, outputs, mutations, writeManifest };
}

test('replaces only the hash assignment, preserving every other byte', () => {
  assert.equal(hashAssignment(CONTENT)[1], OLD_HASH);
  assert.equal(replaceHash(CONTENT, OLD_HASH, NEW_HASH), CONTENT.replace(OLD_HASH, NEW_HASH));
  assert.equal(replaceHash(CONTENT, OLD_HASH, OLD_HASH), CONTENT);
  assert.throws(() => replaceHash(CONTENT, NEW_HASH, OLD_HASH), /original hash/);
  assert.throws(() => hashAssignment(`${CONTENT}\nvendorHash = "${OLD_HASH}";`), /exactly one/);
});

test('rejects noncanonical hashes and executable replacement strings', () => {
  for (const hash of ['', 'sha256-foo', `${NEW_HASH}\n`, '${builtins.readFile /etc/passwd}',
    `sha256-${'A'.repeat(42)}B=`]) {
    assert.equal(validHash(hash), false);
    assert.throws(() => replaceHash(CONTENT, OLD_HASH, hash), /Invalid replacement/);
  }
});

test('rejects extra manifest fields instead of accepting a path or replacement file', t => {
  const f = fixture(t);
  validateUpdate(f.update);
  assert.throws(() => validateUpdate({ ...f.update, path: '.github/workflows/core-build.yml' }), /Unexpected update fields/);
  assert.throws(() => validateUpdate({ ...f.update, runId: '34' }), /Invalid runId/);
  assert.throws(() => validateUpdate({ ...f.update, newHash: OLD_HASH }), /has not changed/);
});

test('finds the artifact by run attempt without relying on workflow_run.pull_requests', async t => {
  const f = fixture(t);
  await findUpdate(f);
  assert.equal(f.outputs['artifact-id'], '78');
});

test('ignores failures with no verified artifact', async t => {
  const f = fixture(t);
  f.github.paginate = async method => method === f.github.rest.pulls.list ? [f.pr] : [];
  await findUpdate(f);
  assert.deepEqual(f.outputs, {});
});

test('commits one hash change with an atomic expected-head condition', async t => {
  const f = fixture(t);
  await applyUpdate(f);
  assert.equal(f.mutations.length, 1);
  const { input } = f.mutations[0].variables;
  assert.equal(input.expectedHeadOid, HEAD);
  assert.deepEqual(input.branch, { repositoryNameWithOwner: 'SurgeDM/Surge', branchName: 'dependency-update' });
  assert.deepEqual(Object.keys(input.fileChanges), ['additions']);
  assert.equal(input.fileChanges.additions.length, 1);
  assert.equal(input.fileChanges.additions[0].path, 'package.nix');
  assert.equal(Buffer.from(input.fileChanges.additions[0].contents, 'base64').toString('utf8'), CONTENT.replace(OLD_HASH, NEW_HASH));
});

test('skips forks, closed PRs, advanced heads, and the default branch', async t => {
  for (const scenario of ['fork', 'closed', 'advanced', 'default']) {
    await t.test(scenario, async t => {
      const f = fixture(t);
      if (scenario === 'fork') f.run.head_repository.id = 99;
      if (scenario === 'closed') f.pr.state = 'closed';
      if (scenario === 'advanced') f.pr.head.sha = 'c'.repeat(40);
      if (scenario === 'default') f.pr.head.ref = f.run.head_branch = 'main';
      await findUpdate(f);
      await applyUpdate(f);
      assert.deepEqual(f.outputs, {});
      assert.equal(f.mutations.length, 0);
    });
  }
});

test('rejects artifacts from another PR, commit, run, or attempt', async t => {
  for (const [field, value] of [['prNumber', 13], ['headSha', 'c'.repeat(40)], ['runId', 35], ['runAttempt', 2]]) {
    await t.test(field, async t => {
      const f = fixture(t);
      f.update[field] = value;
      f.writeManifest();
      await assert.rejects(applyUpdate(f), /Artifact belongs to a different/);
      assert.equal(f.mutations.length, 0);
    });
  }
});

test('rejects a different source workflow and an outdated rerun attempt', async t => {
  const f = fixture(t);
  f.run.path = '.github/workflows/other.yml';
  await assert.rejects(findUpdate(f), /Unexpected source workflow/);
  f.run.path = '.github/workflows/nix-check.yml';
  f.run.run_attempt = 2;
  await assert.rejects(findUpdate(f), /newer run attempt/);
  assert.equal(f.mutations.length, 0);
});

test('rejects oversized artifacts and symlinked manifests', async t => {
  const f = fixture(t);
  f.github.paginate = async method => method === f.github.rest.pulls.list ? [f.pr]
    : [{ id: 78, name: 'nix-vendor-hash-1', size_in_bytes: 100000 }];
  await assert.rejects(findUpdate(f), /unexpectedly large/);
  const manifest = path.join(f.directory, 'update.json');
  fs.renameSync(manifest, path.join(f.directory, 'other.json'));
  fs.symlinkSync('other.json', manifest);
  await assert.rejects(applyUpdate(f), /Invalid update manifest/);
  assert.equal(f.mutations.length, 0);
});

test('rejects an original hash mismatch rather than replacing arbitrary content', async t => {
  const f = fixture(t);
  f.file.content = Buffer.from(CONTENT.replace(OLD_HASH, NEW_HASH)).toString('base64');
  await assert.rejects(applyUpdate(f), /original hash/);
  assert.equal(f.mutations.length, 0);
});

test('rejects malformed UTF-8 rather than changing bytes outside the hash', async t => {
  const f = fixture(t);
  f.file.content = Buffer.concat([Buffer.from(CONTENT), Buffer.from([0xff])]).toString('base64');
  await assert.rejects(applyUpdate(f), /valid UTF-8/);
  assert.equal(f.mutations.length, 0);
});

test('does not retry a failed atomic commit against a newer branch head', async t => {
  const f = fixture(t);
  let attempts = 0;
  f.github.graphql = async (_query, { input }) => {
    attempts++;
    assert.equal(input.expectedHeadOid, HEAD);
    throw new Error('Branch head changed');
  };
  await assert.rejects(applyUpdate(f), /Branch head changed/);
  assert.equal(attempts, 1);
});
