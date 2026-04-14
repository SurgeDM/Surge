import { defineBackground } from 'wxt/utils/define-background';
import { normalizeToken, normalizeServerUrl } from './popup/lib/utils';
import { DownloadStatus, HistoryEntry } from './popup/store/types';
import { STORAGE_KEYS } from '../lib/storage';
import {
  buildDownloadRequestBody,
  buildEventStreamHeaders,
  buildPortScanCandidates,
  coerceStoredBoolean,
  extractPathInfo,
  filterPendingDuplicates,
  findReachableCandidate,
  queueDuplicateDownload,
  resolveInterceptEnabled,
  type PendingDup,
} from '../lib/background-logic';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_PORT = 1700;
const MAX_PORT_SCAN = 100;
const PORT_SCAN_BATCH_SIZE = 20;
const BASE_URL_RETRY_COOLDOWN_MS = 5_000;
const HEADER_EXPIRY_MS = 120_000;
const HEALTH_CHECK_INTERVAL_MS = 5_000;
const SYNC_INTERVAL_MS = 60_000;
const SSE_RETRY_BASE_MS = 3_000;
const SSE_RETRY_MAX_MS = 30_000;

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

let cachedServerUrl: string | null = null;
let cachedDiscoveredServerUrl: string | null = null;
let resolvedBaseUrl: string | null = null;
let cachedAuthToken: string | null = null;
let hasHydratedServerUrl = false;
let hasHydratedDiscoveredServerUrl = false;
let hasHydratedAuthToken = false;
let persistedStatePromise: Promise<void> | null = null;
let isConnected = false;
let lastHealthCheck = 0;
let lastBaseUrlFailureAt = 0;
let sseAbortController: AbortController | null = null;
let baseUrlResolutionPromise: Promise<string | null> | null = null;
let sseRetryCount = 0;

// Stale headers captured during requests. Cleaned up on access + periodically.
const capturedHeaders = new Map<string, { headers: Record<string, string>; timestamp: number }>();

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
  return coerceStoredBoolean(result[key]);
}

async function storageSet(key: string, value: string | boolean): Promise<void> {
  await browser.storage.local.set({ [key]: value });
}

function setCachedAuthTokenState(token: string | null): void {
  cachedAuthToken = token;
  hasHydratedAuthToken = true;
}

function setCachedServerUrlState(url: string | null): void {
  cachedServerUrl = url;
  hasHydratedServerUrl = true;
  resolvedBaseUrl = null;
}

function setCachedDiscoveredServerUrlState(url: string | null): void {
  cachedDiscoveredServerUrl = url;
  hasHydratedDiscoveredServerUrl = true;
}

async function loadPersistedState(): Promise<void> {
  const [token, serverUrl, discoveredServerUrl] = await Promise.all([
    storageGet(STORAGE_KEYS.TOKEN),
    storageGet(STORAGE_KEYS.SERVER_URL),
    storageGet(STORAGE_KEYS.DISCOVERED_SERVER_URL),
  ]);

  if (!hasHydratedAuthToken) {
    setCachedAuthTokenState(token || null);
  }
  if (!hasHydratedServerUrl) {
    setCachedServerUrlState(serverUrl || null);
  }
  if (!hasHydratedDiscoveredServerUrl) {
    setCachedDiscoveredServerUrlState(discoveredServerUrl || null);
  }
}

function ensurePersistedStateLoaded(): Promise<void> {
  if (hasHydratedAuthToken && hasHydratedServerUrl && hasHydratedDiscoveredServerUrl) {
    return Promise.resolve();
  }

  if (!persistedStatePromise) {
    persistedStatePromise = loadPersistedState().catch((error) => {
      persistedStatePromise = null;
      throw error;
    });
  }

  return persistedStatePromise;
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
      const freshEntries = filterPendingDuplicates(entries);
      for (const [id, data] of freshEntries) {
        pendingDuplicates.set(id, data);
        const num = parseInt(id.replace('dup_', ''), 10);
        if (!isNaN(num) && num > pendingDuplicateCounter) pendingDuplicateCounter = num;
      }
      if (freshEntries.length !== entries.length) await persistPendingDuplicates();
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

async function persistDiscoveredServerUrl(url: string | null): Promise<void> {
  setCachedDiscoveredServerUrlState(url);
  await storageSet(STORAGE_KEYS.DISCOVERED_SERVER_URL, url || '');
}

// ---------------------------------------------------------------------------
// URL resolution
// ---------------------------------------------------------------------------

async function discoverBaseUrl(): Promise<string | null> {
  // Try the user-configured URL first and only.
  if (cachedServerUrl) {
    const ok = await healthCheck(cachedServerUrl);
    if (ok) return cachedServerUrl;
    return null;
  }

  const candidates = buildPortScanCandidates(
    DEFAULT_PORT,
    MAX_PORT_SCAN,
    [cachedDiscoveredServerUrl],
  );

  const found = await findReachableCandidate(candidates, healthCheck, PORT_SCAN_BATCH_SIZE);
  return found;
}

async function getBaseUrl(): Promise<string | null> {
  await ensurePersistedStateLoaded();
  if (resolvedBaseUrl) return resolvedBaseUrl;
  if (baseUrlResolutionPromise) return baseUrlResolutionPromise;
  if (Date.now() - lastBaseUrlFailureAt < BASE_URL_RETRY_COOLDOWN_MS) return null;

  baseUrlResolutionPromise = (async () => {
    const nextBaseUrl = await discoverBaseUrl();

    if (!nextBaseUrl) {
      resolvedBaseUrl = null;
      isConnected = false;
      lastBaseUrlFailureAt = Date.now();
      return null;
    }

    resolvedBaseUrl = nextBaseUrl;
    isConnected = true;
    lastBaseUrlFailureAt = 0;

    if (!cachedServerUrl && cachedDiscoveredServerUrl !== nextBaseUrl) {
      await persistDiscoveredServerUrl(nextBaseUrl).catch(() => { });
    }

    return nextBaseUrl;
  })().finally(() => {
    baseUrlResolutionPromise = null;
  });

  return baseUrlResolutionPromise;
}

async function healthCheck(url: string): Promise<boolean> {
  try {
    const resp = await fetch(`${url}/health`, { signal: AbortSignal.timeout(300) });
    if (resp.ok) { isConnected = true; return true; }
  } catch { /* ignore */ }
  if (resolvedBaseUrl === url) resolvedBaseUrl = null;
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
  await ensurePersistedStateLoaded();
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
    const response = await fetch(`${base}${url}`, {
      ...options,
      headers: { 'Content-Type': 'application/json', ...(await authHeaders()), ...(options?.headers || {}) },
      signal: AbortSignal.timeout(5000),
    });
    // Return response even if not .ok so callers can check status codes (like 401)
    return response;
  } catch {
    if (resolvedBaseUrl === base) {
      resolvedBaseUrl = null;
      lastHealthCheck = 0;
    }
    return null;
  }
}

async function fetchDownloadsList(): Promise<{ data: DownloadStatus[]; authError: boolean; ok: boolean }> {
  const resp = await apiFetch('/list');
  if (!resp) return { data: [], authError: false, ok: false };
  if (resp.status === 401) return { data: [], authError: true, ok: false };
  if (!resp.ok) return { data: [], authError: false, ok: false };
  try {
    const j = await resp.json();
    return { data: Array.isArray(j) ? j : [], authError: false, ok: true };
  } catch {
    return { data: [], authError: false, ok: false };
  }
}

async function fetchHistoryList(): Promise<{ data: HistoryEntry[]; authError: boolean; ok: boolean }> {
  const resp = await apiFetch('/history');
  if (!resp) return { data: [], authError: false, ok: false };
  if (resp.status === 401) return { data: [], authError: true, ok: false };
  if (!resp.ok) return { data: [], authError: false, ok: false };
  try {
    const j = await resp.json();
    return { data: Array.isArray(j) ? j : [], authError: false, ok: true };
  } catch {
    return { data: [], authError: false, ok: false };
  }
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
  options?: { skipApproval?: boolean },
): Promise<{ success: boolean; filename?: string; error?: string }> {
  const base = await getBaseUrl();
  if (!base) return { success: false, error: 'Server not running' };

  try {
    const resp = await fetch(`${base}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(await authHeaders()) },
      body: JSON.stringify(buildDownloadRequestBody({
        url,
        filename,
        directory,
        headers,
        skipApproval: options?.skipApproval,
      })),
      signal: AbortSignal.timeout(5000),
    });

    if (resp.ok) {
      const data = await resp.json().catch(() => ({}));
      return { success: true, filename: data.filename };
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
// Download interception
// ---------------------------------------------------------------------------

async function isInterceptEnabled(): Promise<boolean> {
  return resolveInterceptEnabled(await storageGetBoolean(STORAGE_KEYS.INTERCEPT));
}

async function isNotificationsEnabled(): Promise<boolean> {
  const enabled = await storageGetBoolean(STORAGE_KEYS.NOTIFICATIONS);
  return enabled !== false; // Default to true if undefined
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

function updateBadge(): void {
  const count = pendingDuplicates.size;
  browser.action.setBadgeText({ text: count > 0 ? count.toString() : '' });
  if (count > 0) browser.action.setBadgeBackgroundColor({ color: '#FF0000' });
}

async function tryOpenPopup(): Promise<void> {
  try { await browser.action.openPopup(); } catch { /* ignore — requires focused window */ }
}

async function isDuplicateDownload(url: string): Promise<boolean> {
  const { data: list } = await fetchDownloadsList();
  const normalized = url.replace(/\/$/, '');
  return list.some(dl => (dl.url || '').replace(/\/$/, '') === normalized);
}

async function handleDownloadCreated(downloadItem: {
  id: number; url: string; filename?: string; state?: string; startTime?: string;
}): Promise<void> {
  if (!await isInterceptEnabled()) return;
  if (shouldSkipUrl(downloadItem.url)) return;
  if (!isFreshDownload(downloadItem)) return;

  // Cancel the browser download IMMEDIATELY — before any network calls.
  // This is the critical fix: when the browser window is focused, Chrome actively
  // progresses the download during any async work. Cancelling first ensures we
  // always win the race regardless of window focus state.
  try {
    await browser.downloads.cancel(downloadItem.id);
    await browser.downloads.erase({ id: downloadItem.id } as any);
  } catch { /* already completed or removed — ignore */ }

  if (!await checkHealthSilent()) return;

  const { filename, directory } = extractPathInfo(downloadItem);
  const duplicateDisplayName = filename || downloadItem.url.split('/').pop()?.split('?')[0] || 'Unknown file';
  const headers = getCapturedHeaders(downloadItem.url) ?? {};

  // Check for duplicates in the extension BEFORE sending to server.
  // This way the TUI never sees a duplicate prompt.
  if (await isDuplicateDownload(downloadItem.url)) {
    pendingDuplicateCounter = await queueDuplicateDownload({
      pendingDuplicates,
      pendingDuplicateCounter,
      url: downloadItem.url,
      filename: duplicateDisplayName,
      directory,
      cleanupStaleDuplicates,
      persistPendingDuplicates,
      updateBadge,
      openPopup: tryOpenPopup,
      sendPrompt: message => browser.runtime.sendMessage(message),
    });
    return;
  }

  // Force empty filename hint for backend - rely on backend prober.
  const result = await sendToSurge(downloadItem.url, '', directory, headers);

  if (result.success) {
    await tryOpenPopup();
    if (await isNotificationsEnabled()) {
      browser.notifications.create({
        type: 'basic',
        iconUrl: 'icons/icon48.png',
        title: 'Surge',
        message: `Download started: ${result.filename || 'Unknown file'}`,
      });
    }
  } else if (result.error) {
    if (await isNotificationsEnabled()) {
      browser.notifications.create({
        type: 'basic',
        iconUrl: 'icons/icon48.png',
        title: 'Surge Error',
        message: `Failed to start download: ${result.error}`,
      });
    }
  }
}

// ---------------------------------------------------------------------------
// SSE event stream
// ---------------------------------------------------------------------------

async function startSSEStream(): Promise<void> {
  sseAbortController?.abort();
  sseAbortController = new AbortController();

  const base = await getBaseUrl();
  if (!base) {
    scheduleSSERetry();
    return;
  }

  try {
    const resp = await fetch(`${base}/events`, {
      headers: buildEventStreamHeaders(cachedAuthToken),
      signal: sseAbortController.signal,
    });
    if (!resp.ok || !resp.body) {
      scheduleSSERetry();
      return;
    }

    // Connected — reset retry backoff
    sseRetryCount = 0;

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    let currentEvent: string | null = null;
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        if (buffer.trim()) {
          for (const line of buffer.split('\n')) {
            if (line.startsWith('event: ')) currentEvent = line.slice(7).trim();
            else if (line.startsWith('data: ') && currentEvent) {
              try {
                const data = JSON.parse(line.slice(6));
                browser.runtime.sendMessage({ type: 'sseEvent', event: currentEvent, data }).catch(() => { });
              } catch { /* skip malformed */ }
            }
          }
        }
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        if (line.startsWith('event: ')) currentEvent = line.slice(7).trim();
        else if (line.startsWith('data: ') && currentEvent) {
          try {
            const data = JSON.parse(line.slice(6));
            browser.runtime.sendMessage({ type: 'sseEvent', event: currentEvent, data }).catch(() => { });
          } catch { /* skip malformed */ }
          currentEvent = null;
        }
      }
    }
  } catch { /* stream closed or aborted */ }

  scheduleSSERetry();
}

function scheduleSSERetry(): void {
  const delay = Math.min(SSE_RETRY_BASE_MS * Math.pow(2, sseRetryCount), SSE_RETRY_MAX_MS);
  sseRetryCount++;
  setTimeout(() => startSSEStream().catch(() => { }), delay);
}

async function fullSync(): Promise<void> {
  if (!await checkHealthSilent()) return;
  const [downloadsResult, historyResult] = await Promise.all([fetchDownloadsList(), fetchHistoryList()]);
  browser.runtime.sendMessage({
    type: 'syncUpdate',
    downloads: downloadsResult.data,
    history: historyResult.data,
    authError: downloadsResult.authError || historyResult.authError,
    authValid: downloadsResult.ok || historyResult.ok,
  }).catch(() => { });
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

function handleMessage(message: Record<string, any>): Promise<unknown> | unknown {
  switch (message.type) {
    // Health / connection
    case 'checkHealth': return checkHealthSilent().then(healthy => ({ healthy }));

    case 'validateAuth':
      return (async () => {
        const base = await getBaseUrl();
        if (!base) return { ok: false, error: 'no_server' };

        const token = normalizeToken(message.token || '');
        const headers = token ? { Authorization: `Bearer ${token}` } : await authHeaders();
        try {
          const resp = await fetch(`${base}/list`, {
            headers,
            signal: AbortSignal.timeout(3000),
          });
          return resp.ok ? { ok: true } : { ok: false, status: resp.status };
        } catch (e) { return { ok: false, error: String(e) }; }
      })();

    // Downloads / history
    case 'getDownloads':
      return (async () => {
        const { data, authError, ok } = await fetchDownloadsList();
        return { downloads: data, authError, authValid: ok, connected: isConnected };
      })();

    case 'getHistory':
      return (async () => {
        const { data, authError, ok } = await fetchHistoryList();
        return { history: data.slice(0, 100), authError, authValid: ok, connected: isConnected };
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
        return { success: r !== null && r.ok };
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
  if (id) browser.runtime.sendMessage({ type: 'promptDuplicate', id, filename: data.filename }).catch(() => { });
}

async function handleConfirmDuplicate(id: string): Promise<{ success: boolean; error?: string }> {
  const pending = pendingDuplicates.get(id);
  if (!pending) return { success: false, error: 'Pending download not found' };

  pendingDuplicates.delete(id);
  await persistPendingDuplicates();
  updateBadge();

  const result = await sendToSurge(
    pending.url,
    '', // Force empty - rely on backend prober
    pending.directory,
    {},
    { skipApproval: true },
  );
  if (result.success) {
    if (await isNotificationsEnabled()) {
      browser.notifications.create({
        type: 'basic',
        iconUrl: 'icons/icon48.png',
        title: 'Surge',
        message: `Download started: ${pending.filename}`,
      });
    }
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

  // Storage change propagation
  browser.storage.onChanged.addListener((changes, areaName) => {
    if (areaName !== 'local') return;
    if (changes[STORAGE_KEYS.SERVER_URL]?.newValue !== undefined) {
      setCachedServerUrlState(normalizeServerUrl(changes[STORAGE_KEYS.SERVER_URL].newValue as string) || null);
      lastHealthCheck = 0;
      lastBaseUrlFailureAt = 0;
    }
    if (changes[STORAGE_KEYS.TOKEN]?.newValue !== undefined) {
      setCachedAuthTokenState(normalizeToken(changes[STORAGE_KEYS.TOKEN].newValue as string) || null);
    }
    if (changes[STORAGE_KEYS.DISCOVERED_SERVER_URL]?.newValue !== undefined) {
      setCachedDiscoveredServerUrlState(normalizeServerUrl(changes[STORAGE_KEYS.DISCOVERED_SERVER_URL].newValue as string) || null);
    }
  });

  // Header capture — Firefox doesn't support the extraHeaders permission
  const isFF = (browser.runtime.getURL as (path?: string) => string)('').startsWith('moz-extension:');
  const listenerOptions: Parameters<typeof browser.webRequest.onBeforeSendHeaders.addListener>[2] = ['requestHeaders'];
  if (!isFF) listenerOptions.push('extraHeaders');
  browser.webRequest.onBeforeSendHeaders.addListener(
    (details) => {
      captureHeaders(details as any);
      return undefined;
    },
    { urls: ['<all_urls>'] },
    listenerOptions,
  );

  // Message handler — Chrome MV3 requires sendResponse + return true for async responses.
  // Returning a Promise from onMessage does NOT work without webextension-polyfill.
  browser.runtime.onMessage.addListener(((
    message: Record<string, any>,
    _sender: unknown,
    sendResponse: (response: unknown) => void,
  ) => {
    const result = handleMessage(message);
    if (result instanceof Promise) {
      result.then(sendResponse).catch(() => sendResponse({ error: 'internal error' }));
      return true; // Keep message channel open for async response
    }
    sendResponse(result);
    return true;
  }) as Parameters<typeof browser.runtime.onMessage.addListener>[0]);

  // Health check — start SSE stream when connection is established
  setInterval(async () => {
    const wasConnected = isConnected;
    await checkHealthSilent();
    if (!isConnected && wasConnected) sseAbortController?.abort();
    if (isConnected && !wasConnected) startSSEStream().catch(() => { });
  }, HEALTH_CHECK_INTERVAL_MS);

  // Periodic full sync with backend
  setInterval(() => { fullSync().catch(() => { }); }, SYNC_INTERVAL_MS);

  // Startup: restore persisted state
  rehydratePendingDuplicates().catch(() => { });
  ensurePersistedStateLoaded()
    .then(() => checkHealthSilent())
    .then(() => {
      if (isConnected) startSSEStream().catch(() => { });
    })
    .catch(() => { });
});

export const __test__ = {
  ensurePersistedStateLoaded,
  getCachedState(): {
    authToken: string | null;
    serverUrl: string | null;
    discoveredServerUrl: string | null;
  } {
    return {
      authToken: cachedAuthToken,
      serverUrl: cachedServerUrl,
      discoveredServerUrl: cachedDiscoveredServerUrl,
    };
  },
  setCachedAuthToken(token: string | null): void {
    setCachedAuthTokenState(token);
  },
  resetState(): void {
    cachedServerUrl = null;
    cachedDiscoveredServerUrl = null;
    resolvedBaseUrl = null;
    cachedAuthToken = null;
    hasHydratedServerUrl = false;
    hasHydratedDiscoveredServerUrl = false;
    hasHydratedAuthToken = false;
    persistedStatePromise = null;
    isConnected = false;
    lastHealthCheck = 0;
    lastBaseUrlFailureAt = 0;
    sseAbortController = null;
    baseUrlResolutionPromise = null;
    sseRetryCount = 0;
    capturedHeaders.clear();
    pendingDuplicateCounter = 0;
    pendingDuplicates.clear();
    processedIds.clear();
  },
  handleDownloadCreated,
};
