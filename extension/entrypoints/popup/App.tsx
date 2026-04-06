import { onMount, onCleanup } from 'solid-js';
import {
  serverConnected,
  setServerConnected,
  activeDownloads,
  setActiveDownloads,
  setHistoryDownloads,
  currentView,
  setCurrentView,
  setInterceptEnabled,
  handleSseEvent,
  setServerUrl,
  setAuthToken,
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

  async function sendMessage<T>(message: RuntimeMessage): Promise<T> {
    return browser.runtime.sendMessage(message) as Promise<T>;
  }

  async function loadSettings(): Promise<void> {
    try {
      const serverUrlResponse = await sendMessage<{ url?: string }>({ type: 'getServerUrl' });
      if (serverUrlResponse?.url !== undefined) setServerUrl(serverUrlResponse.url);

      const authResponse = await sendMessage<{ token?: string; verified?: boolean }>({ type: 'getAuthToken' });
      if (authResponse?.token !== undefined) {
        setAuthToken(authResponse.token);
        setAuthValid(authResponse.verified === true);
      }

      const statusResponse = await sendMessage<{ enabled?: boolean }>({ type: 'getStatus' });
      if (statusResponse) setInterceptEnabled(statusResponse.enabled !== false);
    } catch { /* ignore */ }
  }

  async function fetchDownloads(): Promise<void> {
    try {
      const response = await sendMessage<{
        downloads?: DownloadStatus[];
        connected?: boolean;
        authError?: boolean;
      }>({ type: 'getDownloads' });

      if (response?.downloads) {
        setServerConnected(response.connected === true);
        setActiveDownloads(response.downloads);
      }
      if (response?.authError) setAuthValid(false);

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
        scheduleRefresh();
        break;
      case 'syncUpdate':
        if (Array.isArray(message.downloads)) setActiveDownloads(message.downloads as DownloadStatus[]);
        if (Array.isArray(message.history)) setHistoryDownloads(message.history as HistoryEntry[]);
        break;
      case 'serverStatus':
        if (typeof message.connected === 'boolean') setServerConnected(message.connected);
        break;
    }
  }

  onMount(async () => {
    await loadSettings();
    await fetchDownloads();

    pollInterval = setInterval(() => { void fetchDownloads(); }, DOWNLOAD_POLL_MS);
    healthInterval = setInterval(async () => {
      try {
        const response = await sendMessage<{ healthy?: boolean }>({ type: 'checkHealth' });
        if (response && typeof response.healthy === 'boolean') setServerConnected(response.healthy);
      } catch {
        setServerConnected(false);
      }
    }, HEALTH_POLL_MS);

    browser.runtime.onMessage.addListener(onMessageListener as Parameters<typeof browser.runtime.onMessage.addListener>[0]);
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
          <img src="/icons/icon48.png" alt="Surge" />
          <h1>SURGE</h1>
        </div>
        <div class="header-right">
          <StatusBadge connected={serverConnected()} />
        </div>
      </header>

      <section class="downloads-section">
        <DownloadList activeDownloads={activeDownloads()} onViewChange={handleViewChange} />
      </section>

      <DuplicateModal />
    </div>
  );
}
