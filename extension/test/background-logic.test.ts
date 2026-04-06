import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildDownloadRequestBody,
  buildEventStreamHeaders,
  coerceStoredBoolean,
  extractPathInfo,
  filterPendingDuplicates,
  openEventStream,
  queueDuplicateDownload,
  resolveInterceptEnabled,
} from '../lib/background-logic';

describe('background logic', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('coerces stored booleans from extension storage values', () => {
    expect(coerceStoredBoolean(true)).toBe(true);
    expect(coerceStoredBoolean(false)).toBe(false);
    expect(coerceStoredBoolean('true')).toBe(true);
    expect(coerceStoredBoolean('false')).toBe(false);
    expect(coerceStoredBoolean(undefined)).toBeUndefined();
    expect(resolveInterceptEnabled(undefined)).toBe(true);
    expect(resolveInterceptEnabled(false)).toBe(false);
  });

  it('includes auth when building SSE headers', () => {
    expect(buildEventStreamHeaders('secret-token')).toEqual({
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
      Authorization: 'Bearer secret-token',
    });

    expect(buildEventStreamHeaders(null)).toEqual({
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
    });
  });

  it('builds download request bodies with optional skip approval', () => {
    expect(buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      directory: '/downloads',
      headers: { Cookie: 'a=b' },
      skipApproval: true,
    })).toEqual({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      path: '/downloads',
      headers: { Cookie: 'a=b' },
      skip_approval: true,
    });

    expect(buildDownloadRequestBody({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      directory: '',
      headers: {},
    })).toEqual({
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      headers: undefined,
      skip_approval: undefined,
    });
  });

  it('updates the badge when queueing a duplicate download', async () => {
    const pendingDuplicates = new Map();
    const persistPendingDuplicates = vi.fn().mockResolvedValue(undefined);
    const updateBadge = vi.fn();
    const openPopup = vi.fn().mockResolvedValue(undefined);
    const sendPrompt = vi.fn().mockResolvedValue(undefined);

    const nextCounter = await queueDuplicateDownload({
      pendingDuplicates,
      pendingDuplicateCounter: 0,
      url: 'https://example.com/file.zip',
      filename: 'file.zip',
      directory: '/downloads',
      persistPendingDuplicates,
      updateBadge,
      openPopup,
      sendPrompt,
      now: () => 1234,
    });

    expect(nextCounter).toBe(1);
    expect([...pendingDuplicates]).toEqual([[
      'dup_1',
      {
        url: 'https://example.com/file.zip',
        filename: 'file.zip',
        directory: '/downloads',
        timestamp: 1234,
      },
    ]]);
    expect(persistPendingDuplicates).toHaveBeenCalledOnce();
    expect(updateBadge).toHaveBeenCalledOnce();
    expect(openPopup).toHaveBeenCalledOnce();
    expect(sendPrompt).toHaveBeenCalledWith({
      type: 'promptDuplicate',
      id: 'dup_1',
      filename: 'file.zip',
    });
  });

  it('opens the SSE stream with the expected headers', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response('ok', { status: 200 }));
    const controller = new AbortController();

    await openEventStream('http://127.0.0.1:1700', 'token-123', controller.signal, fetchImpl);

    expect(fetchImpl).toHaveBeenCalledWith('http://127.0.0.1:1700/events', {
      headers: {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        Authorization: 'Bearer token-123',
      },
      signal: controller.signal,
    });
  });

  it('filters expired pending duplicates during rehydration', () => {
    expect(filterPendingDuplicates([
      ['dup_1', { url: 'a', filename: 'a', directory: '', timestamp: 50 }],
      ['dup_2', { url: 'b', filename: 'b', directory: '', timestamp: 150 }],
    ], 200, 100)).toEqual([
      ['dup_2', { url: 'b', filename: 'b', directory: '', timestamp: 150 }],
    ]);
  });

  it('extracts filename and directory across path styles', () => {
    expect(extractPathInfo({ filename: 'C:\\Users\\me\\file.zip' })).toEqual({
      filename: 'file.zip',
      directory: 'C:/Users/me',
    });
    expect(extractPathInfo({ filename: '/home/me/file.zip' })).toEqual({
      filename: 'file.zip',
      directory: '/home/me',
    });
    expect(extractPathInfo({ filename: 'downloads/file with spaces.zip' })).toEqual({
      filename: 'file with spaces.zip',
      directory: 'downloads',
    });
    expect(extractPathInfo({ filename: 'plain.txt' })).toEqual({
      filename: 'plain.txt',
      directory: '',
    });
  });
});
