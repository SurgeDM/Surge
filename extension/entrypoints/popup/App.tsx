import { onMount, onCleanup } from 'solid-js';
import { readStoredBoolean, readStoredString, STORAGE_KEYS } from '../../lib/storage';
import {
  serverConnected,
  setServerConnected,
  activeDownloads,
  reconcileActiveDownloads,
  setHistoryDownloads,
  currentView,
  setCurrentView,
  setInterceptEnabled,
  setNotificationsEnabled,
  handleSseEvent,
  setServerUrl,
  setServerUrlLocked,
  setAuthToken,
  setAuthTokenLocked,
  authValid,
  setAuthValid,
} from './store';
import StatusBadge from './components/StatusBadge';
import DownloadList from './components/DownloadList';
import DuplicateModal from './components/DuplicateModal';
import './popup.css';
import type { DownloadStatus, HistoryEntry } from './store/types';
import type { ViewMode } from './store';

const DOWNLOAD_POLL_MS = 15_000;
const HEALTH_POLL_MS = 3_000;
const SSE_REFRESH_DEBOUNCE_MS = 2_000;

type RuntimeMessage = Record<string, unknown>;

export default function App() {
  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let healthInterval: ReturnType<typeof setInterval> | null = null;
  let refreshDebounceTimer: ReturnType<typeof setTimeout> | null = null;


  function scheduleRefresh(): void {
    if (refreshDebounceTimer) return;
    refreshDebounceTimer = setTimeout(() => {
      refreshDebounceTimer = null;
      void fetchDownloads();
    }, SSE_REFRESH_DEBOUNCE_MS);
  }

  function shouldRefreshAfterSseEvent(event: string): boolean {
    return event !== 'progress';
  }

  async function sendMessage<T>(message: RuntimeMessage): Promise<T> {
    return browser.runtime.sendMessage(message) as Promise<T>;
  }

  async function loadSettings(): Promise<void> {
    try {
      const storedValues = await browser.storage.local.get([
        STORAGE_KEYS.SERVER_URL,
        STORAGE_KEYS.TOKEN,
        STORAGE_KEYS.VERIFIED,
        STORAGE_KEYS.INTERCEPT,
        STORAGE_KEYS.NOTIFICATIONS,
      ]);

      const storedServerUrl = readStoredString(storedValues, STORAGE_KEYS.SERVER_URL);
      const storedToken = readStoredString(storedValues, STORAGE_KEYS.TOKEN);

      setServerUrl(storedServerUrl);
      setServerUrlLocked(storedServerUrl.trim().length > 0);

      setAuthToken(storedToken);
      setAuthTokenLocked(storedToken.trim().length > 0);
      setAuthValid(readStoredBoolean(storedValues, STORAGE_KEYS.VERIFIED, false));

      setInterceptEnabled(readStoredBoolean(storedValues, STORAGE_KEYS.INTERCEPT, true));
      setNotificationsEnabled(readStoredBoolean(storedValues, STORAGE_KEYS.NOTIFICATIONS, true));
    } catch { /* ignore */ }
  }

  async function fetchDownloads(): Promise<void> {
    try {
      const response = await sendMessage<{
        downloads?: DownloadStatus[];
        connected?: boolean;
        authError?: boolean;
        authValid?: boolean;
      }>({ type: 'getDownloads' });


      if (response?.downloads) {
        setServerConnected(response.connected === true);
        reconcileActiveDownloads(response.downloads);
      }
      if (response?.authError) setAuthValid(false);
      else if (response?.authValid) setAuthValid(true);

      if (response?.connected && currentView() === 'history') {
        await fetchHistory();
      }
    } catch {
      setServerConnected(false);
    }
  }

  async function fetchHistory() {
    try {
      const response = await sendMessage<{ history?: HistoryEntry[] }>({ type: 'getHistory' });
      if (response?.history) {
        setHistoryDownloads(response.history);
      }
    } catch { /* ignore */ }
  }

  const handleViewChange = (view: ViewMode) => {
    setCurrentView(view);
    if (view === 'history') void fetchHistory();
  };

  function onMessageListener(message: RuntimeMessage): void {
    switch (message.type) {
      case 'sseEvent':
        handleSseEvent(String(message.event), message.data);
        if (shouldRefreshAfterSseEvent(String(message.event))) scheduleRefresh();
        break;
      case 'syncUpdate':
        if (Array.isArray(message.downloads)) reconcileActiveDownloads(message.downloads as DownloadStatus[]);
        if (Array.isArray(message.history)) setHistoryDownloads(message.history as HistoryEntry[]);
        if (message.authError === true) setAuthValid(false);
        else if (message.authValid === true) setAuthValid(true);
        break;
      case 'serverStatus':
        if (typeof message.connected === 'boolean') setServerConnected(message.connected);
        break;
    }
  }

  onMount(async () => {
    browser.runtime.onMessage.addListener(onMessageListener as Parameters<typeof browser.runtime.onMessage.addListener>[0]);


    void loadSettings();

    // Fire an immediate health check BEFORE fetchDownloads so we establish connection first
    try {
      const healthResp = await sendMessage<{ healthy?: boolean }>({ type: 'checkHealth' });
      if (healthResp && typeof healthResp.healthy === 'boolean') {
        setServerConnected(healthResp.healthy);
      }
    } catch { /* ignore */ }

    void fetchDownloads();

    pollInterval = setInterval(() => {
      if (!serverConnected()) {
        void fetchDownloads();
        return;
      }

      if (currentView() === 'history') {
        void fetchHistory();
      }
    }, DOWNLOAD_POLL_MS);
    healthInterval = setInterval(async () => {
      try {
        const response = await sendMessage<{ healthy?: boolean }>({ type: 'checkHealth' });
        if (response && typeof response.healthy === 'boolean') setServerConnected(response.healthy);
      } catch {
        setServerConnected(false);
      }
    }, HEALTH_POLL_MS);

  });

  onCleanup(() => {
    if (pollInterval) clearInterval(pollInterval);
    if (healthInterval) clearInterval(healthInterval);
    if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
    browser.runtime.onMessage.removeListener(onMessageListener as Parameters<typeof browser.runtime.onMessage.removeListener>[0]);
  });

  return (
    <div class="container">
      <header class="header">
        <div class="logo">
          <div class="logo-mark-wrap">
            <img src="/icons/icon48.png" alt="Surge" class="logo-mark" />
          </div>
          <div class="logo-wordmark" aria-label="Surge">
            <span class="logo-word">Surge</span>
            <span class="logo-cursor">_</span>
          </div>
        </div>
        <div class="header-right">
          <StatusBadge
            connected={serverConnected()}
            authValid={authValid()}
            onClick={() => handleViewChange('settings')}
          />
        </div>
      </header>

      <section class="downloads-section">
        <DownloadList activeDownloads={activeDownloads()} onViewChange={handleViewChange} />
      </section>

      <DuplicateModal />
    </div>
  );
}
