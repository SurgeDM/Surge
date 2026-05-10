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

  it('skips healthy local servers that reject the token during discovery', async () => {
    storageGet.mockResolvedValue({});

    const fetchImpl = vi.fn(async (url: string) => {
      if (url === 'http://127.0.0.1:1700/health' || url === 'http://127.0.0.1:1701/health') {
        return new Response('ok', { status: 200 });
      }
      if (url === 'http://127.0.0.1:1700/list') {
        return new Response('Unauthorized', { status: 401 });
      }
      if (url === 'http://127.0.0.1:1701/list') {
        return new Response('[]', { status: 200 });
      }
      throw new Error('connection refused');
    });
    vi.stubGlobal('fetch', fetchImpl);

    const result = await __test__.discoverBaseUrlForToken('good-token');

    expect(result).toEqual({
      base: 'http://127.0.0.1:1701',
      sawUnauthorized: true,
      sawReachable: true,
    });
    expect(fetchImpl).toHaveBeenCalledWith('http://127.0.0.1:1700/list', {
      headers: { Authorization: 'Bearer good-token' },
      signal: expect.any(AbortSignal),
    });
  });

  it('reports unauthorized discovery separately from no-server discovery', async () => {
    storageGet.mockResolvedValue({});

    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (url === 'http://127.0.0.1:1700/health') {
        return new Response('ok', { status: 200 });
      }
      if (url === 'http://127.0.0.1:1700/list') {
        return new Response('Unauthorized', { status: 401 });
      }
      throw new Error('connection refused');
    }));

    const result = await __test__.discoverBaseUrlForToken('bad-token');

    expect(result).toEqual({
      base: null,
      sawUnauthorized: true,
      sawReachable: true,
    });
  });
});
