import { defineBackground } from 'wxt/sandbox';
import { normalizeToken, normalizeServerUrl } from './popup/lib/utils';
import { DownloadStatus, HistoryEntry } from './popup/store/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_PORT = 1700;
const MAX_PORT_SCAN = 100;
const HEADER_EXPIRY_MS = 120_000;
const HEALTH_CHECK_INTERVAL_MS = 5_000;
const SYNC_INTERVAL_MS = 60_000;

const STORAGE_KEYS = {
  INTERCEPT: 'interceptEnabled',
  TOKEN: 'authToken',
  VERIFIED: 'authVerified',
  SERVER_URL: 'serverUrl',
} as const;

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

let cachedServerUrl: string | null = null;
let cachedAuthToken: string | null = null;
let isConnected = false;
let lastHealthCheck = 0;
let sseAbortController: AbortController | null = null;

// Stale headers captured during requests. Cleaned up on access + periodically.
const capturedHeaders = new Map<string, { headers: Record<string, string>; timestamp: number }>();

// Pending duplicate downloads that need user confirmation.
// Persisted to storage so they survive Chrome MV3 service worker restarts.
interface PendingDup {
  url: string;
  filename: string;
  directory: string;
  timestamp: number;
}
const PENDING_DUP_KEY = 'pendingDuplicates';
let pendingDuplicateCounter = 0;
const pendingDuplicates = new Map<string, PendingDup>();

// Dedupes rapid onCreated events for the same browser download ID.
const processedIds = new Set<number>();

// ---------------------------------------------------------------------------
// Storage helpers
// ---------------------------------------------------------------------------

async function storageGet(key: string): Promise<string | undefined> {
  const result = await browser.storage.local.get(key);
  return typeof result[key] === 'string' ? result[key] : undefined;
}

async function storageGetBoolean(key: string): Promise<boolean | undefined> {
  const result = await browser.storage.local.get(key);
  const value = result[key];
  if (typeof value === 'boolean') return value;
  if (value === 'true') return true;
  if (value === 'false') return false;
  return undefined;
}

async function storageSet(key: string, value: string | boolean): Promise<void> {
  await browser.storage.local.set({ [key]: value });
}

async function loadPersistedState(): Promise<void> {
  const [token, serverUrl] = await Promise.all([
    storageGet(STORAGE_KEYS.TOKEN),
    storageGet(STORAGE_KEYS.SERVER_URL),
  ]);
  cachedAuthToken = token || null;
  cachedServerUrl = serverUrl || null;
}

// ---------------------------------------------------------------------------
// Pending duplicates persistence
// ---------------------------------------------------------------------------

async function persistPendingDuplicates(): Promise<void> {
  await browser.storage.local.set({ [PENDING_DUP_KEY]: [...pendingDuplicates] });
}

async function rehydratePendingDuplicates(): Promise<void> {
  try {
    const result = await browser.storage.local.get(PENDING_DUP_KEY);
    const entries = result[PENDING_DUP_KEY] as [string, PendingDup][] | undefined;
    if (entries?.length) {
      for (const [id, data] of entries) {
        pendingDuplicates.set(id, data);
        const num = parseInt(id.replace('dup_', ''), 10);
        if (!isNaN(num) && num > pendingDuplicateCounter) pendingDuplicateCounter = num;
      }
      updateBadge();
    }
  } catch { /* ignore */ }
}

function cleanupStaleDuplicates(): void {
  const cutoff = Date.now() - 60_000;
  for (const [id, data] of pendingDuplicates) {
    if (data.timestamp < cutoff) pendingDuplicates.delete(id);
  }
}

// ---------------------------------------------------------------------------
// URL resolution
// ---------------------------------------------------------------------------

async function getBaseUrl(): Promise<string | null> {
  // Try the user-configured URL first
  if (cachedServerUrl) {
    if (await healthCheck(cachedServerUrl)) return cachedServerUrl;
    isConnected = false;
    return null;
  }

  // Fall back to port scanning
  for (let p = DEFAULT_PORT; p < DEFAULT_PORT + MAX_PORT_SCAN; p++) {
    const url = `http://127.0.0.1:${p}`;
    if (await healthCheck(url)) return url;
  }

  isConnected = false;
  return null;
}

async function healthCheck(url: string): Promise<boolean> {
  try {
    const resp = await fetch(`${url}/health`, { signal: AbortSignal.timeout(300) });
    if (resp.ok) { isConnected = true; return true; }
  } catch { /* ignore */ }
  return false;
}

async function checkHealthSilent(): Promise<boolean> {
  const now = Date.now();
  if (now - lastHealthCheck < 1000) return isConnected;
  lastHealthCheck = now;
  const url = await getBaseUrl();
  isConnected = url !== null;
  return isConnected;
}

async function authHeaders(): Promise<Record<string, string>> {
  if (!cachedAuthToken) return {};
  return { Authorization: `Bearer ${cachedAuthToken}` };
}

// ---------------------------------------------------------------------------
// API helpers
// ---------------------------------------------------------------------------

async function apiFetch(url: string, options?: RequestInit): Promise<Response | null> {
  const base = await getBaseUrl();
  if (!base) return null;
  try {
    return await fetch(`${base}${url}`, {
      ...options,
      headers: { 'Content-Type': 'application/json', ...(await authHeaders()), ...(options?.headers || {}) },
      signal: AbortSignal.timeout(5000),
    }).then(r => r.ok ? r : null);
  } catch { return null; }
}

async function fetchDownloadsList(): Promise<DownloadStatus[]> {
  const resp = await apiFetch('/list');
  if (!resp) return [];
  try { const j = await resp.json(); return Array.isArray(j) ? j : []; } catch { return []; }
}

async function fetchHistoryList(): Promise<HistoryEntry[]> {
  const resp = await apiFetch('/history');
  if (!resp) return [];
  try { const j = await resp.json(); return Array.isArray(j) ? j : []; } catch { return []; }
}

/**
 * Send a download request to the Surge backend.
 * Returns { success: true } or { success: false, error: string }.
 */
async function sendToSurge(
  url: string,
  filename: string,
  directory: string,
  headers: Record<string, string>,
): Promise<{ success: boolean; error?: string }> {
  const base = await getBaseUrl();
  if (!base) return { success: false, error: 'Server not running' };

  const body: Record<string, unknown> = {
    url,
    filename,
    headers: Object.keys(headers).length > 0 ? headers : undefined,
  };
  if (directory) body.path = directory;

  try {
    const resp = await fetch(`${base}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(await authHeaders()) },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(5000),
    });

    if (resp.ok) return { success: true };

    if (resp.status === 409) {
      const text = await resp.text().catch(() => '');
      try { const j = JSON.parse(text); return { success: false, error: j.message || text }; }
      catch { return { success: false, error: text }; }
    }

    return { success: false, error: await resp.text().catch(() => '') };
  } catch (error) {
    return { success: false, error: error instanceof Error ? error.message : String(error) };
  }
}

// ---------------------------------------------------------------------------
// Header capture (webRequest.onBeforeSendHeaders)
// ---------------------------------------------------------------------------

function captureHeaders(details: { url?: string; requestHeaders?: { name: string; value?: string }[] }): void {
  if (!details.requestHeaders || !details.url) return;
  const headers: Record<string, string> = {};
  for (const h of details.requestHeaders) {
    if (h.value) headers[h.name] = h.value;
  }
  if (Object.keys(headers).length > 0) {
    capturedHeaders.set(details.url, { headers, timestamp: Date.now() });
    if (capturedHeaders.size > 1000) cleanupExpiredHeaders();
  }
}

function cleanupExpiredHeaders(): void {
  const now = Date.now();
  for (const [url, data] of capturedHeaders) {
    if (now - data.timestamp > HEADER_EXPIRY_MS) capturedHeaders.delete(url);
  }
}

function getCapturedHeaders(url: string): Record<string, string> | null {
  const data = capturedHeaders.get(url);
  if (!data || Date.now() - data.timestamp > HEADER_EXPIRY_MS) {
    capturedHeaders.delete(url);
    return null;
  }
  return data.headers;
}

// ---------------------------------------------------------------------------
// Path extraction
// ---------------------------------------------------------------------------

/**
 * Parse the browser-provided downloadItem.filename into {filename, directory}.
 * Handles Windows paths (C:\foo\bar.txt) and POSIX paths (/home/foo/bar.txt).
 */
function extractPathInfo(downloadItem: { filename?: string }): { filename: string; directory: string } {
  if (!downloadItem.filename) return { filename: '', directory: '' };

  const normalized = downloadItem.filename.replace(/\\/g, '/');
  const parts = normalized.split('/');
  const filename = parts.pop() || '';

  let directory = '';
  if (parts.length > 0) {
    if (/^[A-Za-z]:$/.test(parts[0])) {
      // Windows: C:/foo/bar => C:/foo
      directory = parts.join('/');
    } else if (parts[0] === '') {
      // POSIX absolute: /foo/bar => /foo
      directory = '/' + parts.slice(1).join('/');
    } else {
      // Relative path — treat the whole thing as directory
      directory = parts.join('/');
    }
  }

  return { filename, directory };
}

// ---------------------------------------------------------------------------
// Download interception
// ---------------------------------------------------------------------------

async function isInterceptEnabled(): Promise<boolean> {
  const val = await storageGetBoolean(STORAGE_KEYS.INTERCEPT);
  return val ?? true;
}

function shouldSkipUrl(url: string): boolean {
  return url.startsWith('blob:')
    || url.startsWith('data:')
    || url.startsWith('chrome-extension:')
    || url.startsWith('moz-extension:');
}

function isFreshDownload(item: { state?: string; startTime?: string }): boolean {
  if (item.state && item.state !== 'in_progress') return false;
  if (!item.startTime) return true;
  return Date.now() - new Date(item.startTime).getTime() <= 30_000;
}

async function isDuplicateDownload(url: string): Promise<boolean> {
  const list = await fetchDownloadsList();
  const normalized = url.replace(/\/$/, '');
  return list.some(dl => (dl.url || '').replace(/\/$/, '') === normalized);
}

function updateBadge(): void {
  const count = pendingDuplicates.size;
  browser.action.setBadgeText({ text: count > 0 ? count.toString() : '' });
  if (count > 0) browser.action.setBadgeBackgroundColor({ color: '#FF0000' });
}

async function handleDownloadCreated(downloadItem: {
  id: number; url: string; filename?: string; state?: string; startTime?: string;
}): Promise<void> {
  if (!await isInterceptEnabled()) return;
  if (shouldSkipUrl(downloadItem.url)) return;
  if (!isFreshDownload(downloadItem)) return;

  // Duplicate: cancel browser download, queue for user confirmation
  if (await isDuplicateDownload(downloadItem.url)) {
    try {
      await browser.downloads.cancel(downloadItem.id);
      await browser.downloads.erase({ id: downloadItem.id } as any);
    } catch { /* ignore */ }

    const { filename, directory } = extractPathInfo(downloadItem);
    const displayName = filename || downloadItem.url.split('/').pop() || 'Unknown file';

    const pendingId = `dup_${++pendingDuplicateCounter}`;
    pendingDuplicates.set(pendingId, {
      url: downloadItem.url,
      filename: displayName,
      directory,
      timestamp: Date.now(),
    });
    cleanupStaleDuplicates();
    await persistPendingDuplicates();
    updateBadge();

    try { await browser.action.openPopup(); } catch { /* ignore */ }
    browser.runtime.sendMessage({ type: 'promptDuplicate', id: pendingId, filename: displayName }).catch(() => {});
    return;
  }

  // Fresh download: send to Surge
  if (!await checkHealthSilent()) return;

  const { filename, directory } = extractPathInfo(downloadItem);
  const headers = getCapturedHeaders(downloadItem.url) ?? {};

  try {
    await browser.downloads.cancel(downloadItem.id);
    await browser.downloads.erase({ id: downloadItem.id } as any);

    const result = await sendToSurge(downloadItem.url, filename, directory, headers);
    if (result.success) {
      browser.notifications.create({
        type: 'basic',
        iconUrl: 'icons/icon48.png',
        title: 'Surge',
        message: `Download started: ${filename || downloadItem.url.split('/').pop()}`,
      });
      try { await browser.action.openPopup(); } catch { /* ignore */ }
    } else {
      browser.notifications.create({
        type: 'basic',
        iconUrl: 'icons/icon48.png',
        title: 'Surge Error',
        message: `Failed to start download: ${result.error}`,
      });
    }
  } catch (error) {
    console.error('[Surge] Failed to intercept:', error);
  }
}

// ---------------------------------------------------------------------------
// SSE event stream
// ---------------------------------------------------------------------------

async function startSSEStream(): Promise<void> {
  sseAbortController?.abort();
  sseAbortController = new AbortController();

  const base = await getBaseUrl();
  if (!base) return;

  try {
    const resp = await fetch(`${base}/events`, {
      headers: {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        ...(await authHeaders()),
      },
      signal: sseAbortController.signal,
    });
    if (!resp.ok || !resp.body) {
      setTimeout(() => startSSEStream().catch(() => {}), 3000);
      return;
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      let currentEvent: string | null = null;
      for (const line of lines) {
        if (line.startsWith('event: ')) currentEvent = line.slice(7).trim();
        else if (line.startsWith('data: ') && currentEvent) {
          try {
            const data = JSON.parse(line.slice(6));
            browser.runtime.sendMessage({ type: 'sseEvent', event: currentEvent, data }).catch(() => {});
          } catch { /* skip malformed */ }
          currentEvent = null;
        }
      }
    }
  } catch { /* stream closed */ }

  setTimeout(() => startSSEStream().catch(() => {}), 3000);
}

async function fullSync(): Promise<void> {
  if (!await checkHealthSilent()) return;
  const [downloads, history] = await Promise.all([fetchDownloadsList(), fetchHistoryList()]);
  browser.runtime.sendMessage({ type: 'syncUpdate', downloads, history }).catch(() => {});
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

function handleMessage(message: Record<string, any>): Promise<unknown> | unknown {
  switch (message.type) {
    // Health / connection
    case 'checkHealth': return checkHealthSilent().then(healthy => ({ healthy }));
    case 'getStatus': return isInterceptEnabled().then(enabled => ({ enabled }));

    // Auth
    case 'getAuthToken':
      return Promise.all([
        Promise.resolve(cachedAuthToken || ''),
        storageGet(STORAGE_KEYS.VERIFIED).then(v => v === 'true'),
      ]).then(([token, verified]) => ({ token, verified }));

    case 'setAuthToken': {
      const normalized = normalizeToken(message.token || '');
      return storageSet(STORAGE_KEYS.TOKEN, normalized).then(async () => {
        cachedAuthToken = normalized;
        await storageSet(STORAGE_KEYS.VERIFIED, 'false');
        return { success: true };
      }).catch(() => ({ success: false, error: 'Failed to persist auth token' }));
    }

    case 'getAuthVerified':
      return storageGet(STORAGE_KEYS.VERIFIED).then(v => ({ verified: v === 'true' }));

    case 'setAuthVerified':
      return storageSet(STORAGE_KEYS.VERIFIED, message.verified === true ? 'true' : 'false')
        .then(() => ({ success: true }));

    case 'validateAuth':
      return (async () => {
        const base = await getBaseUrl();
        if (!base) return { ok: false, error: 'no_server' };
        try {
          const resp = await fetch(`${base}/list`, {
            headers: await authHeaders(),
            signal: AbortSignal.timeout(3000),
          });
          return resp.ok ? { ok: true } : { ok: false, status: resp.status };
        } catch (e) { return { ok: false, error: String(e) }; }
      })();

    // Settings
    case 'setStatus':
      return storageSet(STORAGE_KEYS.INTERCEPT, message.enabled).then(() => ({ success: true }));

    case 'getServerUrl':
      return storageGet(STORAGE_KEYS.SERVER_URL).then(url => ({ url: url || '' }));

    case 'setServerUrl': {
      const normalized = normalizeServerUrl(message.url || '');
      return storageSet(STORAGE_KEYS.SERVER_URL, normalized).then(() => {
        cachedServerUrl = normalized || null;
        lastHealthCheck = 0;
        return { success: true };
      });
    }

    // Downloads / history
    case 'getDownloads':
      return (async () => {
        const d = await fetchDownloadsList();
        return { downloads: d, authError: false, connected: isConnected };
      })();

    case 'getHistory':
      return (async () => {
        const h = await fetchHistoryList();
        return { history: h.slice(0, 100), authError: false, connected: isConnected };
      })();

    // Download actions
    case 'pauseDownload':
    case 'resumeDownload':
    case 'cancelDownload':
    case 'openFile':
    case 'openFolder': {
      const methodMap: Record<string, string> = {
        pauseDownload: 'POST',
        resumeDownload: 'POST',
        cancelDownload: 'DELETE',
        openFile: 'POST',
        openFolder: 'POST',
      };
      const pathMap: Record<string, string> = {
        pauseDownload: `/pause?id=${message.id}`,
        resumeDownload: `/resume?id=${message.id}`,
        cancelDownload: `/delete?id=${message.id}`,
        openFile: `/open-file?id=${encodeURIComponent(message.id)}`,
        openFolder: `/open-folder?id=${encodeURIComponent(message.id)}`,
      };
      return (async () => {
        const r = await apiFetch(pathMap[message.type], { method: methodMap[message.type] });
        return { success: r !== null };
      })();
    }

    // Duplicate confirmation
    case 'confirmDuplicate':
      return handleConfirmDuplicate(message.id);

    case 'skipDuplicate':
      return handleSkipDuplicate(message.id);

    case 'getPendingDuplicates': {
      const dups = [];
      for (const [id, data] of pendingDuplicates) {
        dups.push({ id, filename: data.filename, url: data.url });
      }
      return Promise.resolve({ duplicates: dups });
    }

    default:
      return Promise.resolve({ error: 'Unknown message type' });
  }
}

async function notifyNextPendingDuplicate(): Promise<void> {
  const nextDuplicate = pendingDuplicates.entries().next().value as [string, PendingDup] | undefined;
  if (!nextDuplicate) return;

  const [id, data] = nextDuplicate;
  if (id) browser.runtime.sendMessage({ type: 'promptDuplicate', id, filename: data.filename }).catch(() => {});
}

async function handleConfirmDuplicate(id: string): Promise<{ success: boolean; error?: string }> {
  const pending = pendingDuplicates.get(id);
  if (!pending) return { success: false, error: 'Pending download not found' };

  pendingDuplicates.delete(id);
  await persistPendingDuplicates();
  updateBadge();

  const result = await sendToSurge(pending.url, pending.filename, pending.directory, {});
  if (result.success) {
    browser.notifications.create({
      type: 'basic',
      iconUrl: 'icons/icon48.png',
      title: 'Surge',
      message: `Download started: ${pending.filename}`,
    });
  }
  await notifyNextPendingDuplicate();
  return { success: result.success };
}

async function handleSkipDuplicate(id: string): Promise<{ success: boolean }> {
  pendingDuplicates.delete(id);
  await persistPendingDuplicates();
  updateBadge();
  await notifyNextPendingDuplicate();
  return { success: true };
}

// ---------------------------------------------------------------------------
// Background entry point
// ---------------------------------------------------------------------------

export default defineBackground(() => {
  // Download interception
  browser.downloads.onCreated.addListener((downloadItem: {
    id: number; url: string; filename?: string; state?: string; startTime?: string;
  }) => {
    if (processedIds.has(downloadItem.id)) return;
    processedIds.add(downloadItem.id);
    setTimeout(() => processedIds.delete(downloadItem.id), 120_000);
    handleDownloadCreated(downloadItem).catch(err =>
      console.error('[Surge] Download intercept error:', err),
    );
  });

  // Notification click handler
  browser.notifications.onClicked.addListener((notificationId: string) => {
    if (notificationId.startsWith('surge-confirm-')) {
      try { browser.action.openPopup(); } catch { /* ignore */ }
      browser.notifications.clear(notificationId);
    }
  });

  // Storage change propagation
  browser.storage.onChanged.addListener((changes, areaName) => {
    if (areaName !== 'local') return;
    if (changes[STORAGE_KEYS.SERVER_URL]?.newValue !== undefined) {
      cachedServerUrl = (changes[STORAGE_KEYS.SERVER_URL].newValue as string) || '';
      lastHealthCheck = 0;
    }
    if (changes[STORAGE_KEYS.TOKEN]?.newValue !== undefined) {
      cachedAuthToken = normalizeToken(changes[STORAGE_KEYS.TOKEN].newValue as string) || null;
    }
  });

  // Header capture — Firefox doesn't support the extraHeaders permission
  const isFF = (browser.runtime.getURL as (path?: string) => string)('').startsWith('moz-extension:');
  const listenerOptions: Parameters<typeof browser.webRequest.onBeforeSendHeaders.addListener>[2] = ['requestHeaders'];
  if (!isFF) listenerOptions.push('extraHeaders');
  browser.webRequest.onBeforeSendHeaders.addListener(
    details => captureHeaders(details as any),
    { urls: ['<all_urls>'] },
    listenerOptions,
  );

  // Message handler
  browser.runtime.onMessage.addListener(handleMessage as Parameters<typeof browser.runtime.onMessage.addListener>[0]);

  // Health check — start SSE stream when connection is established
  setInterval(async () => {
    const wasConnected = isConnected;
    await checkHealthSilent();
    if (isConnected && !wasConnected) startSSEStream().catch(() => {});
  }, HEALTH_CHECK_INTERVAL_MS);

  // Periodic full sync with backend
  setInterval(() => { fullSync().catch(() => {}); }, SYNC_INTERVAL_MS);

  // Startup: restore persisted state
  rehydratePendingDuplicates().catch(() => {});
  loadPersistedState().catch(() => {});
  checkHealthSilent().catch(() => {});
});
