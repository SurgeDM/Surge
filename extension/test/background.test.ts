import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('wxt/utils/define-background', () => ({
  defineBackground: (callback: () => void) => callback,
}));

import { __test__ } from '../entrypoints/background';

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('background auth persistence', () => {
  const storageGet = vi.fn();

  beforeEach(() => {
    __test__.resetState();
    storageGet.mockReset();

    (globalThis as typeof globalThis & { browser: unknown }).browser = {
      storage: {
        local: {
          get: storageGet,
          set: vi.fn(),
        },
      },
    } as unknown;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete (globalThis as typeof globalThis & { browser?: unknown }).browser;
  });

  it('waits for persisted state hydration before continuing', async () => {
    const authToken = createDeferred<Record<string, string>>();
    const serverUrl = createDeferred<Record<string, string>>();
    const discoveredServerUrl = createDeferred<Record<string, string>>();

    storageGet.mockImplementation((key: string) => {
      switch (key) {
        case 'authToken':
          return authToken.promise;
        case 'serverUrl':
          return serverUrl.promise;
        case 'discoveredServerUrl':
          return discoveredServerUrl.promise;
        default:
          return Promise.resolve({});
      }
    });

    const hydrationPromise = __test__.ensurePersistedStateLoaded();

    let settled = false;
    void hydrationPromise.then(() => {
      settled = true;
    });

    await Promise.resolve();
    expect(settled).toBe(false);

    authToken.resolve({ authToken: 'persisted-token' });
    serverUrl.resolve({ serverUrl: 'http://127.0.0.1:1700' });
    discoveredServerUrl.resolve({ discoveredServerUrl: 'http://127.0.0.1:1710' });

    await hydrationPromise;

    expect(__test__.getCachedState()).toEqual({
      authToken: 'persisted-token',
      serverUrl: 'http://127.0.0.1:1700',
      discoveredServerUrl: 'http://127.0.0.1:1710',
    });
  });

  it('does not overwrite a newer token with an older in-flight hydration result', async () => {
    const authToken = createDeferred<Record<string, string>>();
    const serverUrl = createDeferred<Record<string, string>>();
    const discoveredServerUrl = createDeferred<Record<string, string>>();

    storageGet.mockImplementation((key: string) => {
      switch (key) {
        case 'authToken':
          return authToken.promise;
        case 'serverUrl':
          return serverUrl.promise;
        case 'discoveredServerUrl':
          return discoveredServerUrl.promise;
        default:
          return Promise.resolve({});
      }
    });

    const hydrationPromise = __test__.ensurePersistedStateLoaded();

    await Promise.resolve();
    __test__.setCachedAuthToken('fresh-token');

    authToken.resolve({ authToken: 'stale-token' });
    serverUrl.resolve({});
    discoveredServerUrl.resolve({});

    await hydrationPromise;

    expect(__test__.getCachedState().authToken).toBe('fresh-token');
  });
});
