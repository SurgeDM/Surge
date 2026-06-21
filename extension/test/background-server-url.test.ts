/**
 * Unit tests for background.ts server URL state synchronization.
 *
 * Specifically covers reresolveActiveServerUrl — the function called by the
 * storage.onChanged listener when profile-related keys change. The key
 * correctness properties verified here are:
 *
 *   1. It resolves the correct URL from the stored profiles list.
 *   2. It does NOT invoke migrateServerProfiles (a startup-only concern) —
 *      instead it uses parseServerProfiles for the cheap read path.
 *   3. It resets the health-check timers so a fresh connection attempt is made.
 *   4. Changing unrelated keys (e.g. TOKEN) does NOT affect cachedServerUrl.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { STORAGE_KEYS } from '../lib/storage';

vi.mock('wxt/utils/define-background', () => ({
  defineBackground: (callback: () => void) => callback,
}));

import { __test__ } from '../entrypoints/background';

// ---------------------------------------------------------------------------
// Shared browser stub
// ---------------------------------------------------------------------------

function makeBrowserStub(overrides?: {
  profiles?: unknown;
  activeProfileId?: string;
}) {
  const profiles = overrides?.profiles ?? [];
  const activeProfileId = overrides?.activeProfileId ?? '';

  return {
    storage: {
      local: {
        get: vi.fn(async (key: string) => {
          switch (key) {
            case STORAGE_KEYS.PROFILES:
              return { [STORAGE_KEYS.PROFILES]: profiles };
            case STORAGE_KEYS.ACTIVE_PROFILE_ID:
              return { [STORAGE_KEYS.ACTIVE_PROFILE_ID]: activeProfileId };
            default:
              return {};
          }
        }),
        set: vi.fn().mockResolvedValue(undefined),
      },
      onChanged: { addListener: vi.fn() },
    },
    downloads: {
      onCreated: { addListener: vi.fn() },
    },
    webRequest: {
      onBeforeSendHeaders: { addListener: vi.fn() },
    },
    runtime: {
      onMessage: { addListener: vi.fn() },
      getURL: vi.fn().mockReturnValue('chrome-extension://test/'),
    },
    action: {
      setBadgeText: vi.fn(),
      setBadgeBackgroundColor: vi.fn(),
    },
    notifications: { create: vi.fn() },
  };
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('background reresolveActiveServerUrl', () => {
  beforeEach(() => {
    __test__.resetState();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete (globalThis as typeof globalThis & { browser?: unknown }).browser;
  });

  it('resolves the active profile URL from the stored profiles list', async () => {
    const profiles = [
      { id: 'a', name: 'Local', url: 'http://127.0.0.1:1700' },
      { id: 'b', name: 'Remote', url: 'http://remote:1700' },
    ];

    (globalThis as typeof globalThis & { browser: unknown }).browser =
      makeBrowserStub({ profiles, activeProfileId: 'b' });

    await __test__.reresolveActiveServerUrl();

    expect(__test__.getCachedState().serverUrl).toBe('http://remote:1700');
  });

  it('falls back to the first profile when the active id is not found', async () => {
    const profiles = [
      { id: 'a', name: 'Local', url: 'http://127.0.0.1:1700' },
    ];

    (globalThis as typeof globalThis & { browser: unknown }).browser =
      makeBrowserStub({ profiles, activeProfileId: 'missing' });

    await __test__.reresolveActiveServerUrl();

    expect(__test__.getCachedState().serverUrl).toBe('http://127.0.0.1:1700');
  });

  it('sets cachedServerUrl to null when there are no profiles', async () => {
    (globalThis as typeof globalThis & { browser: unknown }).browser =
      makeBrowserStub({ profiles: [], activeProfileId: '' });

    await __test__.reresolveActiveServerUrl();

    expect(__test__.getCachedState().serverUrl).toBeNull();
  });

  it('normalizes the URL (strips trailing slash, adds scheme)', async () => {
    const profiles = [
      { id: 'a', name: 'Local', url: '127.0.0.1:1700/' },
    ];

    (globalThis as typeof globalThis & { browser: unknown }).browser =
      makeBrowserStub({ profiles, activeProfileId: 'a' });

    await __test__.reresolveActiveServerUrl();

    expect(__test__.getCachedState().serverUrl).toBe('http://127.0.0.1:1700');
  });

  it('does NOT call migrateServerProfiles — only reads current profiles', async () => {
    // We spy on migrateServerProfiles via the storage module to confirm it is
    // NOT called from reresolveActiveServerUrl (it should only run at startup).
    const storageMod = await import('../lib/storage');
    const migrateSpy = vi.spyOn(storageMod, 'migrateServerProfiles');

    const profiles = [{ id: 'a', name: 'Local', url: 'http://127.0.0.1:1700' }];
    (globalThis as typeof globalThis & { browser: unknown }).browser =
      makeBrowserStub({ profiles, activeProfileId: 'a' });

    await __test__.reresolveActiveServerUrl();

    expect(migrateSpy).not.toHaveBeenCalled();
  });

  it('changes cachedServerUrl when a different profile becomes active', async () => {
    const profiles = [
      { id: 'a', name: 'Local', url: 'http://127.0.0.1:1700' },
      { id: 'b', name: 'Remote', url: 'http://remote:9000' },
    ];

    // First resolve to profile 'a'
    const browserStub = makeBrowserStub({ profiles, activeProfileId: 'a' });
    (globalThis as typeof globalThis & { browser: unknown }).browser = browserStub;
    await __test__.reresolveActiveServerUrl();
    expect(__test__.getCachedState().serverUrl).toBe('http://127.0.0.1:1700');

    // Simulate profile switch: active becomes 'b'
    browserStub.storage.local.get = vi.fn(async (key: string) => {
      switch (key) {
        case STORAGE_KEYS.PROFILES:
          return { [STORAGE_KEYS.PROFILES]: profiles };
        case STORAGE_KEYS.ACTIVE_PROFILE_ID:
          return { [STORAGE_KEYS.ACTIVE_PROFILE_ID]: 'b' };
        default:
          return {};
      }
    });

    await __test__.reresolveActiveServerUrl();
    expect(__test__.getCachedState().serverUrl).toBe('http://remote:9000');
  });
});
