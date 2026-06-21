/**
 * Unit tests for SettingsView profile management logic.
 *
 * The handler functions are tested via the extracted settings-handlers module
 * (popup/lib/settings-handlers.ts) rather than by rendering the SolidJS
 * component, keeping the test environment as plain Node (no DOM/jsdom needed).
 *
 * Each test injects a fake ProfileStore and a fake StorageApi so we can verify:
 *   - that signals are NOT mutated when storage write fails (rollback)
 *   - that invalid URL inputs are rejected before any storage write
 *   - that signals ARE updated correctly after a successful write
 */

import { describe, expect, it, vi } from 'vitest';
import type { ServerProfile } from '../lib/storage';
import {
  persistProfiles,
  handleAddProfile,
  handleSwitchProfile,
  handleDeleteProfile,
  type ProfileStore,
  type StorageApi,
} from '../entrypoints/popup/lib/settings-handlers';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeProfile(id: string, url: string): ServerProfile {
  return { id, name: id, url };
}

function makeStore(initial?: {
  profiles?: ServerProfile[];
  activeId?: string;
}): ProfileStore & {
  profiles: ServerProfile[];
  activeId: string;
  serverUrl: string;
  locked: boolean;
} {
  let profiles = initial?.profiles ?? [];
  let activeId = initial?.activeId ?? '';
  let serverUrl = '';
  let locked = false;

  return {
    // Readable state (for assertions)
    get profiles() { return profiles; },
    get activeId() { return activeId; },
    get serverUrl() { return serverUrl; },
    get locked() { return locked; },
    // ProfileStore interface
    getProfiles: () => profiles,
    getActiveId: () => activeId,
    setProfiles: (p) => { profiles = p; },
    setActiveId: (id) => { activeId = id; },
    setServerUrl: (url) => { serverUrl = url; },
    setServerUrlLocked: (l) => { locked = l; },
  };
}

function okStorage(): StorageApi {
  return { set: vi.fn().mockResolvedValue(undefined) };
}

function failingStorage(): StorageApi {
  return { set: vi.fn().mockRejectedValue(new Error('QuotaExceededError')) };
}

// ---------------------------------------------------------------------------
// persistProfiles
// ---------------------------------------------------------------------------

describe('persistProfiles', () => {
  it('writes storage then updates signals on success', async () => {
    const store = makeStore();
    const storage = okStorage();
    const profiles = [makeProfile('a', 'http://a:1700')];

    await persistProfiles(profiles, 'a', store, storage);

    expect(store.profiles).toEqual(profiles);
    expect(store.activeId).toBe('a');
    expect(store.serverUrl).toBe('http://a:1700');
    expect(store.locked).toBe(true);
    expect(storage.set).toHaveBeenCalledOnce();
  });

  it('does NOT update signals when storage write fails', async () => {
    const initialProfiles = [makeProfile('old', 'http://old:1700')];
    const store = makeStore({ profiles: initialProfiles, activeId: 'old' });
    const storage = failingStorage();

    await expect(
      persistProfiles([makeProfile('new', 'http://new:1700')], 'new', store, storage),
    ).rejects.toThrow();

    // Signals must still reflect the pre-call state
    expect(store.profiles).toEqual(initialProfiles);
    expect(store.activeId).toBe('old');
    expect(store.serverUrl).toBe('');  // never set
    expect(store.locked).toBe(false);  // never set
  });

  it('sets serverUrlLocked to false when there is no active profile', async () => {
    const store = makeStore();
    const storage = okStorage();

    await persistProfiles([], '', store, storage);

    expect(store.serverUrl).toBe('');
    expect(store.locked).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// handleAddProfile
// ---------------------------------------------------------------------------

describe('handleAddProfile', () => {
  it('returns an error without touching storage when the URL is empty', async () => {
    const store = makeStore();
    const storage = okStorage();

    const result = await handleAddProfile({ name: 'Home', url: '' }, store, storage);

    expect(result.ok).toBe(false);
    expect(result.error).toBe('Enter a server URL');
    expect(storage.set).not.toHaveBeenCalled();
    expect(store.profiles).toHaveLength(0);
  });

  it('returns an error without touching storage when the URL is whitespace only', async () => {
    const store = makeStore();
    const storage = okStorage();

    const result = await handleAddProfile({ name: 'Home', url: '   ' }, store, storage);

    expect(result.ok).toBe(false);
    expect(result.error).toBe('Enter a server URL');
    expect(storage.set).not.toHaveBeenCalled();
  });

  it('adds a profile and updates signals on success', async () => {
    const store = makeStore();
    const storage = okStorage();

    const result = await handleAddProfile(
      { name: 'Home', url: '127.0.0.1:1700' },
      store,
      storage,
    );

    expect(result.ok).toBe(true);
    expect(store.profiles).toHaveLength(1);
    expect(store.profiles[0].url).toBe('http://127.0.0.1:1700');
    expect(store.activeId).toBe(store.profiles[0].id);
    expect(store.locked).toBe(true);
    expect(storage.set).toHaveBeenCalledOnce();
  });

  it('does NOT update signals when storage write fails', async () => {
    const store = makeStore();
    const storage = failingStorage();

    const result = await handleAddProfile(
      { name: 'Home', url: 'http://host:1700' },
      store,
      storage,
    );

    expect(result.ok).toBe(false);
    expect(result.error).toBe('Failed to save');
    // Signals must remain at initial state (rollback)
    expect(store.profiles).toHaveLength(0);
    expect(store.activeId).toBe('');
  });

  it('falls back to the URL as profile name when name is empty', async () => {
    const store = makeStore();
    const storage = okStorage();

    await handleAddProfile({ name: '', url: 'http://host:9000' }, store, storage);

    expect(store.profiles[0].name).toBe('http://host:9000');
  });
});

// ---------------------------------------------------------------------------
// handleSwitchProfile
// ---------------------------------------------------------------------------

describe('handleSwitchProfile', () => {
  const existingProfiles = [
    makeProfile('a', 'http://a:1700'),
    makeProfile('b', 'http://b:1700'),
  ];

  it('switches the active profile on success', async () => {
    const store = makeStore({ profiles: existingProfiles, activeId: 'a' });
    const storage = okStorage();

    const result = await handleSwitchProfile('b', store, storage);

    expect(result.ok).toBe(true);
    expect(store.activeId).toBe('b');
    expect(store.serverUrl).toBe('http://b:1700');
  });

  it('does NOT update signals when storage write fails', async () => {
    const store = makeStore({ profiles: existingProfiles, activeId: 'a' });
    const storage = failingStorage();

    const result = await handleSwitchProfile('b', store, storage);

    expect(result.ok).toBe(false);
    // activeId must remain 'a'
    expect(store.activeId).toBe('a');
  });
});

// ---------------------------------------------------------------------------
// handleDeleteProfile
// ---------------------------------------------------------------------------

describe('handleDeleteProfile', () => {
  it('removes the active profile and falls back to the first remaining one', async () => {
    const profiles = [
      makeProfile('a', 'http://a:1700'),
      makeProfile('b', 'http://b:1700'),
    ];
    const store = makeStore({ profiles, activeId: 'a' });
    const storage = okStorage();

    const result = await handleDeleteProfile(store, storage);

    expect(result.ok).toBe(true);
    expect(store.profiles.map((p) => p.id)).toEqual(['b']);
    expect(store.activeId).toBe('b');
  });

  it('results in an empty profile list when the last profile is deleted', async () => {
    const store = makeStore({
      profiles: [makeProfile('only', 'http://only:1700')],
      activeId: 'only',
    });
    const storage = okStorage();

    await handleDeleteProfile(store, storage);

    expect(store.profiles).toHaveLength(0);
    expect(store.activeId).toBe('');
    expect(store.locked).toBe(false);
  });

  it('does NOT update signals when storage write fails', async () => {
    const profiles = [
      makeProfile('a', 'http://a:1700'),
      makeProfile('b', 'http://b:1700'),
    ];
    const store = makeStore({ profiles, activeId: 'a' });
    const storage = failingStorage();

    const result = await handleDeleteProfile(store, storage);

    expect(result.ok).toBe(false);
    // Signals must remain at their initial values
    expect(store.profiles).toHaveLength(2);
    expect(store.activeId).toBe('a');
  });
});
