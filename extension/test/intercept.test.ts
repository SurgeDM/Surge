import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { __test__ } from '../entrypoints/background';

vi.mock('wxt/utils/define-background', () => ({
  defineBackground: (callback: () => void) => callback,
}));

describe('download interception naming', () => {
  const mockFetch = vi.fn();

  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch);
    __test__.resetState();

    // Mock browser APIs
    vi.stubGlobal('browser', {
      storage: {
        local: {
          get: vi.fn().mockImplementation((key: string) => {
            if (key === 'intercept') return Promise.resolve({ intercept: true });
            if (key === 'serverUrl') return Promise.resolve({ serverUrl: 'http://127.0.0.1:1700' });
            return Promise.resolve({});
          }),
          set: vi.fn(),
        },
      },
      downloads: {
        cancel: vi.fn().mockResolvedValue(undefined),
        erase: vi.fn().mockResolvedValue(undefined),
      },
      action: {
        openPopup: vi.fn().mockResolvedValue(undefined),
        setBadgeText: vi.fn(),
        setBadgeBackgroundColor: vi.fn(),
      },
      runtime: {
        getURL: vi.fn().mockReturnValue('chrome-extension://id/'),
        sendMessage: vi.fn().mockResolvedValue(undefined),
      },
      notifications: {
        create: vi.fn(),
      },
    });

    // Pre-hydrate state so we don't wait for discovery
    return __test__.ensurePersistedStateLoaded();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('sends an empty filename to Surge if browser provides no filename, preventing bad URL hints', async () => {
    // 1. Mock health check and download response
    mockFetch.mockImplementation(async (url: string) => {
      if (url.includes('/health')) return { ok: true };
      if (url.includes('/list')) return { ok: true, json: async () => [] };
      if (url.includes('/download')) return {
        ok: true,
        json: async () => ({ status: 'queued', id: '123', filename: 'resolved-by-backend.zip' })
      };
      return { ok: false };
    });

    const downloadItem = {
      id: 123,
      url: 'https://example.com/some/long/path/with/potential/bad/fallback',
      startTime: new Date().toISOString(),
    };

    await __test__.handleDownloadCreated(downloadItem);

    // 2. Verify sendToSurge was called with EMPTY filename
    const downloadCall = mockFetch.mock.calls.find(call => call[0].includes('/download'));
    expect(downloadCall).toBeDefined();

    const body = JSON.parse(downloadCall?.[1].body);
    expect(body.filename).toBe('');
    expect(body.url).toBe(downloadItem.url);

    // 3. Verify notification uses RESTRICTED resolved filename from backend response
    expect(browser.notifications.create).toHaveBeenCalledWith(expect.objectContaining({
      message: 'Download started: resolved-by-backend.zip'
    }));
  });

  it('preserves the filename if the browser actually provides one', async () => {
    mockFetch.mockImplementation(async (url: string) => {
      if (url.includes('/health')) return { ok: true };
      if (url.includes('/list')) return { ok: true, json: async () => [] };
      if (url.includes('/download')) return {
        ok: true,
        json: async () => ({ status: 'queued', id: '456', filename: 'authoritative.zip' })
      };
      return { ok: false };
    });

    const downloadItem = {
      id: 456,
      url: 'https://example.com/file',
      filename: '/path/to/authoritative.zip', // Browser already knows the name
      startTime: new Date().toISOString(),
    };

    await __test__.handleDownloadCreated(downloadItem);

    const downloadCall = mockFetch.mock.calls.find(call => call[0].includes('/download'));
    const body = JSON.parse(downloadCall?.[1].body);
    expect(body.filename).toBe('');
  });
});
