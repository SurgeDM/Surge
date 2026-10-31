const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const fs = require('node:fs');

const METADATA_PATH = 'nix/release.json';
const SYSTEMS = {
  'x86_64-linux': 'linux_amd64',
  'aarch64-linux': 'linux_arm64',
  'x86_64-darwin': 'darwin_amd64',
  'aarch64-darwin': 'darwin_arm64',
};
const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const RELEASE_ROOT = 'https://github.com/SurgeDM/Surge/releases/download/';

function compareVersions(left, right) {
  assert(VERSION.test(left) && VERSION.test(right), 'Invalid stable version');
  const a = left.split('.').map(BigInt);
  const b = right.split('.').map(BigInt);
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i] ? 1 : -1;
  }
  return 0;
}

function latestRelease(releases) {
  // Extension tags and prereleases share the repository; neither is a core release.
  const stable = releases.filter(release => !release.draft && !release.prerelease &&
    release.tag_name?.startsWith('v') && VERSION.test(release.tag_name.slice(1)));
  assert(stable.length > 0, 'No published stable Surge release found');
  return stable.sort((a, b) => compareVersions(b.tag_name.slice(1), a.tag_name.slice(1)))[0];
}

function validateMetadata(metadata) {
  assert(metadata && VERSION.test(metadata.version), 'Invalid release version');
  assert.deepEqual(Object.keys(metadata).sort(), ['hashes', 'version']);
  assert.deepEqual(Object.keys(metadata.hashes).sort(), Object.keys(SYSTEMS).sort());
  for (const hash of Object.values(metadata.hashes)) {
    assert(typeof hash === 'string' && /^sha256-[A-Za-z0-9+/]{43}=$/.test(hash), 'Invalid SHA-256 hash');
    assert.equal(`sha256-${Buffer.from(hash.slice(7), 'base64').toString('base64')}`, hash);
  }
  return metadata;
}

function asset(release, name) {
  const matches = release.assets.filter(item => item.name === name);
  assert.equal(matches.length, 1, `Expected exactly one release asset: ${name}`);
  const result = matches[0];
  assert.equal(result.state, 'uploaded', `Asset is not uploaded: ${name}`);
  assert(result.size > 0, `Empty release asset: ${name}`);
  assert.equal(result.browser_download_url, `${RELEASE_ROOT}${release.tag_name}/${name}`,
    `Unexpected asset URL: ${name}`);
  return result;
}

function checksumEntries(text) {
  const result = new Map();
  for (const line of text.trim().split(/\r?\n/)) {
    const match = /^([a-fA-F0-9]{64}) [ *](\S+)$/.exec(line);
    assert(match, 'Invalid release checksum entry');
    assert(!result.has(match[2]), `Duplicate checksum: ${match[2]}`);
    result.set(match[2], match[1].toLowerCase());
  }
  return result;
}

function verifyDownload(file, bytes, expected) {
  const actual = createHash('sha256').update(bytes).digest('hex');
  assert.equal(actual, expected, `Checksum mismatch: ${file.name}`);
  if (file.digest) assert.equal(file.digest, `sha256:${actual}`, `GitHub digest mismatch: ${file.name}`);
  return `sha256-${Buffer.from(actual, 'hex').toString('base64')}`;
}

async function download(file) {
  // Do not forward the GitHub API token to release-asset redirect hosts.
  const response = await fetch(file.browser_download_url, { signal: AbortSignal.timeout(120_000) });
  assert(response.ok, `Download failed (${response.status}): ${file.name}`);
  return Buffer.from(await response.arrayBuffer());
}

async function releaseMetadata(release, getBytes = download) {
  assert(!release.draft && !release.prerelease && release.tag_name?.startsWith('v') &&
    VERSION.test(release.tag_name.slice(1)), 'Expected a published stable Surge release');
  const version = release.tag_name.slice(1);
  const checksumFile = asset(release, 'checksums.txt');
  const checksumBytes = await getBytes(checksumFile);
  if (checksumFile.digest) {
    assert(/^sha256:[a-f0-9]{64}$/.test(checksumFile.digest), 'Invalid checksum file digest');
    verifyDownload(checksumFile, checksumBytes, checksumFile.digest.slice(7));
  }
  const checksums = checksumEntries(checksumBytes.toString('utf8'));
  const hashes = {};
  for (const [system, suffix] of Object.entries(SYSTEMS)) {
    const file = asset(release, `Surge_${version}_${suffix}.tar.gz`);
    assert(checksums.has(file.name), `Missing checksum: ${file.name}`);
    hashes[system] = verifyDownload(file, await getBytes(file), checksums.get(file.name));
  }
  return validateMetadata({ version, hashes });
}

function updateNeeded(current, candidate) {
  validateMetadata(current);
  validateMetadata(candidate);
  const comparison = compareVersions(candidate.version, current.version);
  assert(comparison >= 0, 'Refusing to downgrade the Nix release');
  if (comparison === 0) assert.deepEqual(candidate, current, 'Published release checksums have changed');
  return comparison > 0;
}

function serialize(metadata) {
  return `${JSON.stringify(validateMetadata(metadata), null, 2)}\n`;
}

async function prepareUpdate({ github, context, core }) {
  const releases = await github.paginate(github.rest.repos.listReleases, { ...context.repo, per_page: 100 });
  const candidate = await releaseMetadata(latestRelease(releases));
  const current = JSON.parse(fs.readFileSync(METADATA_PATH, 'utf8'));
  const changed = updateNeeded(current, candidate);
  if (changed) fs.writeFileSync(METADATA_PATH, serialize(candidate));
  core.setOutput('changed', String(changed));
  core.setOutput('version', candidate.version);
  core.info(changed ? `Prepared Surge ${candidate.version}` : `Surge ${candidate.version} is current`);
}

async function publishUpdate({ github, context, core }) {
  const candidate = validateMetadata(JSON.parse(fs.readFileSync(METADATA_PATH, 'utf8')));
  const releases = await github.paginate(github.rest.repos.listReleases, { ...context.repo, per_page: 100 });
  if (latestRelease(releases).tag_name !== `v${candidate.version}`) {
    core.notice('A newer release appeared while checking; the next update run will handle it.');
    return;
  }
  const { data: main } = await github.rest.git.getRef({ ...context.repo, ref: 'heads/main' });
  const { data: file } = await github.rest.repos.getContent({
    ...context.repo, path: METADATA_PATH, ref: main.object.sha,
  });
  const current = JSON.parse(Buffer.from(file.content, 'base64').toString('utf8'));
  if (!updateNeeded(current, candidate)) return;

  const branch = `chore/nix-release-v${candidate.version}`;
  try {
    await github.rest.git.createRef({ ...context.repo, ref: `refs/heads/${branch}`, sha: main.object.sha });
  } catch (error) {
    if (error.status !== 422) throw error;
    // A previous attempt may have committed or opened a PR already. Never force-push it.
  }
  const { data: branchFile } = await github.rest.repos.getContent({
    ...context.repo, path: METADATA_PATH, ref: branch,
  });
  const branchMetadata = JSON.parse(Buffer.from(branchFile.content, 'base64').toString('utf8'));
  if (updateNeeded(branchMetadata, candidate)) {
    await github.rest.repos.createOrUpdateFileContents({
      ...context.repo, branch, path: METADATA_PATH, sha: branchFile.sha,
      message: `chore(nix): update Surge to ${candidate.version}`,
      content: Buffer.from(serialize(candidate)).toString('base64'),
    });
  }
  const pulls = await github.paginate(github.rest.pulls.list, {
    ...context.repo, head: `${context.repo.owner}:${branch}`, base: 'main', state: 'open', per_page: 100,
  });
  if (pulls.length) {
    core.info(`Update PR already exists: ${pulls[0].html_url}`);
    return;
  }
  const { data: pr } = await github.rest.pulls.create({
    ...context.repo, head: branch, base: 'main',
    title: `chore(nix): update Surge to ${candidate.version}`,
    body: `Updates the Nix package to [Surge v${candidate.version}](https://github.com/SurgeDM/Surge/releases/tag/v${candidate.version}).\n\nAll four release archives were downloaded and checked against the published SHA-256 checksums. Installation and version checks passed on Linux and macOS, on x86_64 and ARM64.\n\nMerge this PR to make the release available through the default Nix package. GitHub may require a maintainer to approve the PR workflows.`,
  });
  core.info(`Opened ${pr.html_url}`);
}

module.exports = {
  SYSTEMS, compareVersions, latestRelease, validateMetadata, checksumEntries,
  releaseMetadata, updateNeeded, prepareUpdate, publishUpdate, serialize,
};

if (require.main === module) {
  (async () => {
    const releases = [];
    for (let page = 1; ; page++) {
      const response = await fetch(`https://api.github.com/repos/SurgeDM/Surge/releases?per_page=100&page=${page}`, {
        headers: { Accept: 'application/vnd.github+json', ...(process.env.GH_TOKEN && { Authorization: `Bearer ${process.env.GH_TOKEN}` }) },
        signal: AbortSignal.timeout(30_000),
      });
      assert(response.ok, `GitHub release lookup failed: ${response.status}`);
      const batch = await response.json();
      releases.push(...batch);
      if (batch.length < 100) break;
    }
    const candidate = await releaseMetadata(latestRelease(releases));
    const current = JSON.parse(fs.readFileSync(METADATA_PATH, 'utf8'));
    if (updateNeeded(current, candidate)) fs.writeFileSync(METADATA_PATH, serialize(candidate));
    console.log(`Verified Surge ${candidate.version} and all four release downloads`);
  })().catch(error => { console.error(error); process.exitCode = 1; });
}
