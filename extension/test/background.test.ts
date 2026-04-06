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
  const storageSet = vi.fn();

  beforeEach(() => {
    __test__.resetState();
    storageGet.mockReset();
    storageSet.mockReset();

    (globalThis as typeof globalThis & { browser: unknown }).browser = {
      storage: {
        local: {
          get: storageGet,
          set: storageSet,
        },
      },
    } as unknown;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    delete (globalThis as typeof globalThis & { browser?: unknown }).browser;
  });

  it('waits for persisted auth token hydration before responding', async () => {
    const authToken = createDeferred<Record<string, string>>();
    const serverUrl = createDeferred<Record<string, string>>();
    const discoveredServerUrl = createDeferred<Record<string, string>>();
    const authVerified = createDeferred<Record<string, string>>();

    storageGet.mockImplementation((key: string) => {
      switch (key) {
        case 'authToken':
          return authToken.promise;
        case 'serverUrl':
          return serverUrl.promise;
        case 'discoveredServerUrl':
          return discoveredServerUrl.promise;
        case 'authVerified':
          return authVerified.promise;
        default:
          return Promise.resolve({});
      }
    });

    const responsePromise = __test__.handleMessage({ type: 'getAuthToken' }) as Promise<{
      token: string;
      verified: boolean;
    }>;

    let settled = false;
    void responsePromise.then(() => {
      settled = true;
    });

    await Promise.resolve();
    expect(settled).toBe(false);

    authToken.resolve({ authToken: 'persisted-token' });
    serverUrl.resolve({});
    discoveredServerUrl.resolve({});
    await Promise.resolve();
    authVerified.resolve({ authVerified: 'true' });

    await expect(responsePromise).resolves.toEqual({
      token: 'persisted-token',
      verified: true,
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
        case 'authVerified':
          return Promise.resolve({ authVerified: 'false' });
        default:
          return Promise.resolve({});
      }
    });
    storageSet.mockResolvedValue(undefined);

    const hydratedRead = __test__.handleMessage({ type: 'getAuthToken' }) as Promise<{
      token: string;
      verified: boolean;
    }>;

    await Promise.resolve();
    await expect(__test__.handleMessage({ type: 'setAuthToken', token: 'fresh-token' })).resolves.toEqual({
      success: true,
    });

    authToken.resolve({ authToken: 'stale-token' });
    serverUrl.resolve({});
    discoveredServerUrl.resolve({});

    await expect(hydratedRead).resolves.toEqual({
      token: 'fresh-token',
      verified: false,
    });

    await expect(__test__.handleMessage({ type: 'getAuthToken' })).resolves.toEqual({
      token: 'fresh-token',
      verified: false,
    });
  });
});
