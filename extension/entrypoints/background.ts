import { defineBackground } from 'wxt/sandbox';
import { normalizeToken, normalizeServerUrl } from './popup/lib/utils';
import { DownloadStatus, HistoryEntry } from './popup/store/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_PORT = '1700';
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

let cachedPort: string | null = null;
let cachedServerUrl: string | null = null;
let cachedAuthToken: string | null = null;
let isConnected = false;
let lastHealthCheck = 0;
let _healthCheckTimer: ReturnType<typeof setInterval> | null = null;
let _syncIntervalTimer: ReturnType<typeof setInterval> | null = null;
let sseAbortController: AbortController | null = null;

const capturedHeaders = new Map<string, { headers: Record<string, string>; timestamp: number }>();
const pendingDuplicates = new Map<string, { url: string; filename: string; timestamp: number }>();
let pendingDuplicateCounter = 0;
const processedIds = new Set<number>();

// ---------------------------------------------------------------------------
// Storage helpers
// ---------------------------------------------------------------------------

async function storageGet(key: string): Promise<string | undefined> {
  const result = await browser.storage.local.get(key);
  return typeof result[key] === 'string' ? result[key] : undefined;
}

async function storageSet(key: string, value: string | boolean): Promise<void> {
  await browser.storage.local.set({ [key]: value });
}

// ---------------------------------------------------------------------------
// URL resolution
// ---------------------------------------------------------------------------

async function getBaseUrl(): Promise<string | null> {
  if (cachedServerUrl) {
    return isConnected ? cachedServerUrl : null;
  }
  if (cachedPort) {
    try {
      const url = `http://127.0.0.1:${cachedPort}`;
      const resp = await fetch(`${url}/health`, { signal: AbortSignal.timeout(300) });
      if (resp.ok) { isConnected = true; return url; }
    } catch { cachedPort = null; }
  }
  for (let p = parseInt(DEFAULT_PORT); p < parseInt(DEFAULT_PORT) + MAX_PORT_SCAN; p++) {
    try {
      const url = `http://127.0.0.1:${p}`;
      const resp = await fetch(`${url}/health`, { signal: AbortSignal.timeout(200) });
      if (resp.ok) { cachedPort = String(p); isConnected = true; return url; }
    } catch { /* continue */ }
  }
  isConnected = false;
  return null;
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
  const token = cachedAuthToken;
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
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

async function sendToSurge(url: string, filename?: string): Promise<{ success: boolean; error?: string; status?: string }> {
  const base = await getBaseUrl();
  if (!base) return { success: false, error: 'Server not running' };
  const body: Record<string, unknown> = { url, filename: filename || '' };
  try {
    const resp = await fetch(`${base}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(await authHeaders()) },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(5000),
    });
    if (resp.ok) { const d = await resp.json(); return { success: true, status: d?.status }; }
    if (resp.status === 409) {
      const text = await resp.text().catch(() => '');
      try { const j = JSON.parse(text); return { success: false, error: j.message || text }; } catch { return { success: false, error: text }; }
    }
    return { success: false, error: await resp.text().catch(() => '') };
  } catch (error) { return { success: false, error: error instanceof Error ? error.message : String(error) }; }
}

// ---------------------------------------------------------------------------
// Header capture
// ---------------------------------------------------------------------------

function captureHeaders(details: { url?: string; requestHeaders?: { name: string; value?: string }[] | undefined }): void {
  if (!details.requestHeaders || !details.url) return;
  const headers: Record<string, string> = {};
  for (const h of details.requestHeaders) { if (h.value) headers[h.name] = h.value; }
  if (Object.keys(headers).length > 0) {
    capturedHeaders.set(details.url, { headers, timestamp: Date.now() });
    if (capturedHeaders.size > 1000) cleanupExpiredHeaders();
  }
}

function cleanupExpiredHeaders(): void {
  const now = Date.now();
  for (const [url, data] of capturedHeaders) { if (now - data.timestamp > HEADER_EXPIRY_MS) capturedHeaders.delete(url); }
}

function getCapturedHeaders(url: string): Record<string, string> | null {
  const data = capturedHeaders.get(url);
  if (!data || Date.now() - data.timestamp > HEADER_EXPIRY_MS) { capturedHeaders.delete(url); return null; }
  return data.headers;
}

// ---------------------------------------------------------------------------
// Interception helpers
// ---------------------------------------------------------------------------

async function isInterceptEnabled(): Promise<boolean> {
  const val = await storageGet(STORAGE_KEYS.INTERCEPT);
  return val !== 'false';
}

function shouldSkipUrl(url: string): boolean {
  return url.startsWith('blob:') || url.startsWith('data:') || url.startsWith('chrome-extension:') || url.startsWith('moz-extension:');
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

function extractPathInfo(downloadItem: { filename?: string; url: string }): { filename: string; directory: string } {
  let filename = '';
  let directory = '';
  if (downloadItem.filename) {
    const normalized = downloadItem.filename.replace(/\\/g, '/');
    const parts = normalized.split('/');
    filename = parts.pop() || '';
    if (parts.length > 0) {
      if (/^[A-Za-z]:$/.test(parts[0])) directory = parts.join('/');
      else if (parts[0] === '') directory = '/' + parts.slice(1).join('/');
      else directory = parts.join('/');
    }
  }
  return { filename, directory };
}

function updateBadge(): void {
  const count = pendingDuplicates.size;
  browser.action.setBadgeText({ text: count > 0 ? count.toString() : '' });
  if (count > 0) browser.action.setBadgeBackgroundColor({ color: '#FF0000' });
}

// ---------------------------------------------------------------------------
// Download interception
// ---------------------------------------------------------------------------

async function handleDownloadCreated(downloadItem: { id: number; url: string; filename?: string; state?: string; startTime?: string }): Promise<void> {
  const enabled = await isInterceptEnabled();
  if (!enabled) return;
  if (shouldSkipUrl(downloadItem.url)) return;
  if (!isFreshDownload(downloadItem)) return;

  if (await isDuplicateDownload(downloadItem.url)) {
    try {
      await browser.downloads.cancel(downloadItem.id);
      await browser.downloads.erase({ id: downloadItem.id } as any);
    } catch { /* ignore */ }

    const pendingId = `dup_${++pendingDuplicateCounter}`;
    const { filename } = extractPathInfo(downloadItem);
    const displayName = filename || downloadItem.url.split('/').pop() || 'Unknown file';

    pendingDuplicates.set(pendingId, { url: downloadItem.url, filename: displayName, timestamp: Date.now() });
    for (const [id, data] of pendingDuplicates) { if (Date.now() - data.timestamp > 60_000) pendingDuplicates.delete(id); }
    updateBadge();

    try { await browser.action.openPopup(); } catch { /* ignore */ }
    browser.runtime.sendMessage({ type: 'promptDuplicate', id: pendingId, filename: displayName }).catch(() => {});
    return;
  }

  if (!await checkHealthSilent()) return;

  const { filename } = extractPathInfo(downloadItem);
  const _captured = getCapturedHeaders(downloadItem.url);

  try {
    await browser.downloads.cancel(downloadItem.id);
    await browser.downloads.erase({ id: downloadItem.id } as any);

    const result = await sendToSurge(downloadItem.url, filename);
    if (result.success) {
      browser.notifications.create({ type: 'basic', iconUrl: 'icons/icon48.png', title: 'Surge', message: `Download started: ${filename || downloadItem.url.split('/').pop()}` });
      try { await browser.action.openPopup(); } catch { /* ignore */ }
    } else {
      browser.notifications.create({ type: 'basic', iconUrl: 'icons/icon48.png', title: 'Surge Error', message: `Failed to start download: ${result.error}` });
    }
  } catch (error) {
    console.error('[Surge] Failed to intercept:', error);
  }
}

// ---------------------------------------------------------------------------
// SSE stream
// ---------------------------------------------------------------------------

async function startSSEStream(): Promise<void> {
  sseAbortController?.abort();
  sseAbortController = new AbortController();

  const base = await getBaseUrl();
  if (!base) return;

  try {
    const resp = await fetch(`${base}/events`, { headers: { Accept: 'text/event-stream' }, signal: sseAbortController.signal });
    if (!resp.ok || !resp.body) { setTimeout(() => startSSEStream().catch(() => {}), 3000); return; }

    isConnected = true;
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
  if (!(await checkHealthSilent())) return;
  const [downloads, history] = await Promise.all([fetchDownloadsList(), fetchHistoryList()]);
  browser.runtime.sendMessage({ type: 'syncUpdate', downloads, history }).catch(() => {});
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

function handleMessage(message: Record<string, any>): Promise<any> | any {
  switch (message.type) {
    case 'checkHealth': return checkHealthSilent().then(healthy => ({ healthy }));
    case 'getStatus': return isInterceptEnabled().then(enabled => ({ enabled }));
    case 'getAuthToken':
      return Promise.all([Promise.resolve(cachedAuthToken || ''), storageGet(STORAGE_KEYS.VERIFIED).then(v => v === 'true')])
        .then(([token, verified]) => ({ token, verified }));
    case 'setAuthToken': {
      const normalized = normalizeToken(message.token || '');
      return storageSet(STORAGE_KEYS.TOKEN, normalized).then(async () => {
        cachedAuthToken = normalized;
        await storageSet(STORAGE_KEYS.VERIFIED, 'false');
        return { success: true };
      }).catch(() => ({ success: false, error: 'Failed to persist auth token' }));
    }
    case 'getAuthVerified': return storageGet(STORAGE_KEYS.VERIFIED).then(v => ({ verified: v === 'true' }));
    case 'setAuthVerified': return storageSet(STORAGE_KEYS.VERIFIED, message.verified === true ? 'true' : 'false').then(() => ({ success: true }));
    case 'validateAuth': {
      return (async () => {
        const base = await getBaseUrl();
        if (!base) return { ok: false, error: 'no_server' };
        try {
          const resp = await fetch(`${base}/list`, { headers: await authHeaders(), signal: AbortSignal.timeout(3000) });
          return resp.ok ? { ok: true } : { ok: false, status: resp.status };
        } catch (e) { return { ok: false, error: String(e) }; }
      })();
    }
    case 'setStatus': return storageSet(STORAGE_KEYS.INTERCEPT, message.enabled).then(() => ({ success: true }));
    case 'getServerUrl': return storageGet(STORAGE_KEYS.SERVER_URL).then(url => ({ url: url || '' }));
    case 'setServerUrl': {
      const normalized = normalizeServerUrl(message.url || '');
      return storageSet(STORAGE_KEYS.SERVER_URL, normalized).then(() => { cachedServerUrl = normalized || null; lastHealthCheck = 0; return { success: true }; });
    }
    case 'getDownloads': return (async () => { const d = await fetchDownloadsList(); return { downloads: d, authError: false, connected: isConnected }; })();
    case 'getHistory': return (async () => { const h = await fetchHistoryList(); return { history: h.slice(0, 100), authError: false, connected: isConnected }; })();
    case 'pauseDownload':
    case 'resumeDownload':
    case 'cancelDownload':
    case 'openFile':
    case 'openFolder': {
      const methodMap: Record<string, string> = {
        pauseDownload: 'POST', resumeDownload: 'POST', cancelDownload: 'DELETE', openFile: 'POST', openFolder: 'POST',
      };
      const pathMap: Record<string, string> = {
        pauseDownload: `/pause?id=${message.id}`, resumeDownload: `/resume?id=${message.id}`, cancelDownload: `/delete?id=${message.id}`,
        openFile: `/open-file?id=${encodeURIComponent(message.id)}`, openFolder: `/open-folder?id=${encodeURIComponent(message.id)}`,
      };
      return (async () => { const r = await apiFetch(pathMap[message.type], { method: methodMap[message.type] }); return { success: r !== null }; })();
    }

    case 'confirmDuplicate': {
      const pending = pendingDuplicates.get(message.id);
      if (!pending) return Promise.resolve({ success: false, error: 'Pending download not found' });
      pendingDuplicates.delete(message.id);
      updateBadge();
      return sendToSurge(pending.url, pending.filename).then(result => {
        if (result.success) browser.notifications.create({ type: 'basic', iconUrl: 'icons/icon48.png', title: 'Surge', message: `Download started: ${pending.filename}` });
        if (pendingDuplicates.size > 0) {
          const [nid, nd] = pendingDuplicates.entries().next().value!;
          browser.runtime.sendMessage({ type: 'promptDuplicate', id: nid, filename: nd.filename }).catch(() => {});
        }
        return { success: result.success };
      });
    }
    case 'skipDuplicate': {
      pendingDuplicates.delete(message.id);
      updateBadge();
      if (pendingDuplicates.size > 0) {
        const [nid, nd] = pendingDuplicates.entries().next().value!;
        browser.runtime.sendMessage({ type: 'promptDuplicate', id: nid, filename: nd.filename }).catch(() => {});
      }
      return Promise.resolve({ success: true });
    }
    case 'getPendingDuplicates': {
      const dups: { id: string; filename: string; url: string }[] = [];
      for (const [id, data] of pendingDuplicates) dups.push({ id, filename: data.filename, url: data.url });
      return Promise.resolve({ duplicates: dups });
    }
    default: return Promise.resolve({ error: 'Unknown message type' });
  }
}

// ---------------------------------------------------------------------------
// Background entry point (WXT sandbox-compatible)
// ---------------------------------------------------------------------------

export default defineBackground(() => {
  // Download interception
  browser.downloads.onCreated.addListener((downloadItem: browser.downloads.DownloadItem) => {
    if (processedIds.has(downloadItem.id)) return;
    processedIds.add(downloadItem.id);
    setTimeout(() => processedIds.delete(downloadItem.id), 120_000);
    handleDownloadCreated(downloadItem).catch(err => console.error('[Surge] Download intercept error:', err));
  });

  // Notification click
  browser.notifications.onClicked.addListener((notificationId: string) => {
    if (notificationId.startsWith('surge-confirm-')) {
      try { browser.action.openPopup(); } catch { /* ignore */ }
      browser.notifications.clear(notificationId);
    }
  });

  // Storage changes
  browser.storage.onChanged.addListener((changes, areaName) => {
    if (areaName !== 'local') return;
    if (changes[STORAGE_KEYS.SERVER_URL]) { cachedServerUrl = changes[STORAGE_KEYS.SERVER_URL].newValue || ''; lastHealthCheck = 0; }
    if (changes[STORAGE_KEYS.TOKEN]) cachedAuthToken = normalizeToken(changes[STORAGE_KEYS.TOKEN].newValue) || null;
  });

  // Header capture
  const isFF = browser.runtime.getURL('').startsWith('moz-extension:');
  const extraHeaders = isFF ? ([] as any) : (['extraHeaders'] as any);
  browser.webRequest.onBeforeSendHeaders.addListener(
    (details: any) => captureHeaders(details),
    { urls: ['<all_urls>'] },
    ['requestHeaders', ...extraHeaders],
  );

  // Message handler
  browser.runtime.onMessage.addListener(handleMessage);

  // Intervals
  _healthCheckTimer = setInterval(async () => {
    const wasConnected = isConnected;
    await checkHealthSilent();
    if (isConnected && !wasConnected) startSSEStream().catch(() => {});
  }, HEALTH_CHECK_INTERVAL_MS);

  _syncIntervalTimer = setInterval(() => { fullSync().catch(() => {}); }, SYNC_INTERVAL_MS);

  // Initial health check
  checkHealthSilent().catch(() => {});
});
