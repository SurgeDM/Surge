const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const {
  SYSTEMS, compareVersions, latestRelease, validateMetadata, checksumEntries,
  releaseMetadata, updateNeeded, prepareUpdate, publishUpdate,
} = require('./nix-release.cjs');

function fixture(version = '0.12.3', checksumName = 'checksums.txt') {
  const files = new Map();
  const assets = [];
  const lines = [];
  function add(name, bytes) {
    files.set(name, bytes);
    const digest = createHash('sha256').update(bytes).digest('hex');
    assets.push({ name, digest: `sha256:${digest}`, state: 'uploaded', size: bytes.length,
      browser_download_url: `https://github.com/SurgeDM/Surge/releases/download/v${version}/${name}` });
    return digest;
  }
  for (const suffix of Object.values(SYSTEMS)) {
    const name = `Surge_${version}_${suffix}.tar.gz`;
    lines.push(`${add(name, Buffer.from(name))}  ${name}`);
  }
  add(checksumName, Buffer.from(`${lines.join('\n')}\n`));
  return {
    release: { tag_name: `v${version}`, draft: false, prerelease: false, assets },
    getBytes: async file => files.get(file.name), files,
  };
}

async function metadata(version = '0.12.3') {
  const f = fixture(version);
  return releaseMetadata(f.release, f.getBytes);
}

test('selects the highest stable core version, excluding extension and draft releases', () => {
  const releases = [
    { tag_name: 'ext-v99.0.0' }, { tag_name: 'v0.12.4-rc1', prerelease: true },
    { tag_name: 'v0.13.0', draft: true }, { tag_name: 'v0.9.9' },
    { tag_name: 'v0.12.3' }, { tag_name: 'v0.11.5', published_at: '2099-01-01' },
  ];
  assert.equal(latestRelease(releases).tag_name, 'v0.12.3');
  assert.throws(() => latestRelease([{ tag_name: 'ext-v1.0.0' }]), /No published stable/);
  assert(compareVersions('0.12.0', '0.9.0') > 0);
  assert(compareVersions('10000000000000000.0.0', '9999999999999999.0.0') > 0);
  assert.throws(() => compareVersions('01.2.3', '1.2.3'), /Invalid/);
});

test('records verified archive hashes for all four supported systems', async () => {
  const result = await metadata();
  assert.equal(result.version, '0.12.3');
  assert.deepEqual(Object.keys(result.hashes), Object.keys(SYSTEMS));
  const expected = createHash('sha256').update('Surge_0.12.3_linux_amd64.tar.gz').digest('base64');
  assert.equal(result.hashes['x86_64-linux'], `sha256-${expected}`);
  validateMetadata(JSON.parse(fs.readFileSync(path.join(__dirname, '../../nix/release.json'), 'utf8')));
});

test('rejects incomplete releases, duplicate assets, and external URLs', async () => {
  const missing = fixture();
  missing.release.assets.shift();
  await assert.rejects(releaseMetadata(missing.release, missing.getBytes), /exactly one release asset/);
  const duplicate = fixture();
  duplicate.release.assets.push(duplicate.release.assets[0]);
  await assert.rejects(releaseMetadata(duplicate.release, duplicate.getBytes), /exactly one release asset/);
  const external = fixture();
  external.release.assets[0].browser_download_url = 'https://example.com/surge.tar.gz';
  await assert.rejects(releaseMetadata(external.release, external.getBytes), /Unexpected asset URL/);
  const pending = fixture();
  pending.release.assets[0].state = 'new';
  await assert.rejects(releaseMetadata(pending.release, pending.getBytes), /not uploaded/);
});

test('rejects corrupted archives and mismatches with GitHub asset digests', async () => {
  const corrupted = fixture();
  corrupted.files.set(corrupted.release.assets[0].name, Buffer.from('corrupted'));
  await assert.rejects(releaseMetadata(corrupted.release, corrupted.getBytes), /Checksum mismatch/);
  const apiMismatch = fixture();
  apiMismatch.release.assets[0].digest = `sha256:${'0'.repeat(64)}`;
  await assert.rejects(releaseMetadata(apiMismatch.release, apiMismatch.getBytes), /GitHub digest mismatch/);
  const checksumMismatch = fixture();
  const checksum = checksumMismatch.release.assets.at(-1);
  checksumMismatch.files.set(checksum.name, Buffer.from('corrupted checksum file'));
  await assert.rejects(releaseMetadata(checksumMismatch.release, checksumMismatch.getBytes), /Checksum mismatch/);
});

test('requires each archive checksum and rejects malformed or ambiguous entries', async () => {
  const f = fixture();
  const checksum = f.release.assets.at(-1);
  delete checksum.digest;
  f.files.set(checksum.name, Buffer.from(`${'0'.repeat(64)}  unrelated.zip\n`));
  await assert.rejects(releaseMetadata(f.release, f.getBytes), /Missing checksum/);
  assert.throws(() => checksumEntries('not a checksum'), /Invalid release checksum/);
  assert.throws(() => checksumEntries(`${'0'.repeat(64)}  file\n${'1'.repeat(64)}  file`), /Duplicate checksum/);
});

test('requires exactly the supported platforms and canonical SHA-256 hashes', async () => {
  const missing = await metadata();
  delete missing.hashes['aarch64-darwin'];
  assert.throws(() => validateMetadata(missing));
  const invalid = await metadata();
  invalid.hashes['x86_64-linux'] = 'sha256-invalid';
  assert.throws(() => validateMetadata(invalid), /Invalid SHA-256/);
});

test('updates forward, skips unchanged releases, and refuses downgrade or replaced assets', async () => {
  const current = await metadata();
  assert.equal(updateNeeded(current, await metadata('0.12.4')), true);
  assert.equal(updateNeeded(current, await metadata()), false);
  assert.throws(() => updateNeeded(current, { ...current, version: '0.11.2' }), /downgrade/);
  const changed = await metadata();
  changed.hashes['x86_64-linux'] = changed.hashes['aarch64-linux'];
  assert.throws(() => updateNeeded(current, changed), /checksums have changed/);
});

// Publishing uses API mocks: no credentials, network calls, or actual repository writes.
async function publisherFixture(currentVersion = '0.12.2') {
  const candidate = await metadata();
  const current = await metadata(currentVersion);
  const writes = [];
  let existingBranch = false;
  let branchMetadata = current;
  let pulls = [];
  let latest = 'v0.12.3';
  const github = {
    paginate: async (method) => method === 'releases' ? [{ tag_name: latest }] : pulls,
    rest: {
      git: {
        getRef: async () => ({ data: { object: { sha: 'main-sha' } } }),
        createRef: async args => {
          if (existingBranch) throw Object.assign(new Error('Already exists'), { status: 422 });
          writes.push(['branch', args]);
        },
      },
      repos: {
        listReleases: 'releases',
        getContent: async args => ({ data: {
          sha: 'file-sha', content: Buffer.from(JSON.stringify(args.ref === 'main-sha' ? current : branchMetadata)).toString('base64'),
        } }),
        createOrUpdateFileContents: async args => { writes.push(['commit', args]); },
      },
      pulls: {
        list: 'pulls',
        create: async args => { writes.push(['pr', args]); return { data: { html_url: 'https://example.com/pr' } }; },
      },
    },
  };
  return {
    candidate, writes, github,
    context: { repo: { owner: 'SurgeDM', repo: 'Surge' } },
    core: { info() {}, notice() {} },
    existing() { existingBranch = true; branchMetadata = candidate; pulls = [{ html_url: 'https://example.com/pr' }]; },
    newer() { latest = 'v0.12.4'; },
  };
}

async function withMetadata(candidate, callback) {
  const previous = process.cwd();
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'nix-release-test-'));
  fs.mkdirSync(path.join(directory, 'nix'));
  fs.writeFileSync(path.join(directory, 'nix/release.json'), JSON.stringify(candidate));
  try { process.chdir(directory); await callback(); }
  finally { process.chdir(previous); fs.rmSync(directory, { recursive: true, force: true }); }
}

test('publishes only the metadata file on a release-specific branch with a checked file SHA', async () => {
  const f = await publisherFixture();
  await withMetadata(f.candidate, () => publishUpdate(f));
  assert.deepEqual(f.writes.map(([type]) => type), ['branch', 'commit', 'pr']);
  const commit = f.writes[1][1];
  assert.equal(commit.path, 'nix/release.json');
  assert.equal(commit.sha, 'file-sha');
  assert.equal(commit.branch, 'chore/nix-release-v0.12.3');
  assert.deepEqual(JSON.parse(Buffer.from(commit.content, 'base64')), f.candidate);
});

test('repeated publishers do not duplicate commits or PRs', async () => {
  const f = await publisherFixture();
  f.existing();
  await withMetadata(f.candidate, () => publishUpdate(f));
  assert.deepEqual(f.writes, []);
});

test('skips publication if main is already updated or a newer release appeared during checks', async () => {
  const current = await publisherFixture('0.12.3');
  await withMetadata(current.candidate, () => publishUpdate(current));
  assert.deepEqual(current.writes, []);
  const stale = await publisherFixture();
  stale.newer();
  await withMetadata(stale.candidate, () => publishUpdate(stale));
  assert.deepEqual(stale.writes, []);
});

test('failed archive verification leaves the committed metadata untouched', async () => {
  const f = fixture('0.12.4');
  f.files.set(f.release.assets[0].name, Buffer.from('broken'));
  const before = await metadata();
  const originalFetch = global.fetch;
  try {
    global.fetch = async url => ({ ok: true, arrayBuffer: async () => f.files.get(url.split('/').at(-1)) });
    await withMetadata(before, async () => {
      await assert.rejects(prepareUpdate({
        github: { paginate: async () => [f.release], rest: { repos: { listReleases: 'releases' } } },
        context: { repo: {} }, core: { setOutput() {}, info() {} },
      }), /Checksum mismatch/);
      assert.deepEqual(JSON.parse(fs.readFileSync('nix/release.json')), before);
    });
  } finally { global.fetch = originalFetch; }
});

test('supports the legacy versioned checksum filename', async () => {
  const f = fixture('0.12.3', 'Surge_0.12.3_checksums.txt');
  assert.equal((await releaseMetadata(f.release, f.getBytes)).version, '0.12.3');
});
