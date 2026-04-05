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

export default function App() {
  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let healthInterval: ReturnType<typeof setInterval> | null = null;
  let refreshDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  function scheduleRefresh(): void {
    if (refreshDebounceTimer) return;
    refreshDebounceTimer = setTimeout(() => { refreshDebounceTimer = null; fetchDownloads(false); }, 2000);
  }

  async function loadSettings() {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getServerUrl' }) as { url?: string };
      if (res?.url !== undefined) setServerUrl(res.url);

      const tokenRes = await browser.runtime.sendMessage({ type: 'getAuthToken' }) as { token?: string; verified?: boolean };
      if (tokenRes?.token !== undefined) {
        setAuthToken(tokenRes.token);
        if (tokenRes.verified) setAuthValid(true);
      }

      const statusRes = await browser.runtime.sendMessage({ type: 'getStatus' }) as { enabled?: boolean };
      if (statusRes) setInterceptEnabled(statusRes.enabled !== false);
    } catch { /* ignore */ }
  }

  async function fetchDownloads(_full = false) {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getDownloads' }) as { downloads?: DownloadStatus[]; connected?: boolean; authError?: boolean };
      if (res?.downloads) {
        setServerConnected(res.connected || false);
        setActiveDownloads(res.downloads);
      }
      if (res?.authError) setAuthValid(false);

      if (res?.connected && currentView() === 'history') {
        await fetchHistory();
      }
    } catch {
      setServerConnected(false);
    }
  }

  async function fetchHistory() {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getHistory' }) as { history?: HistoryEntry[] };
      if (res?.history) {
        setHistoryDownloads(res.history);
      }
    } catch { /* ignore */ }
  }

  const handleViewChange = (view: ViewMode) => {
    setCurrentView(view);
    if (view === 'history') void fetchHistory();
  };

  function onMessageListener(message: Record<string, unknown>) {
    if (message.type === 'sseEvent') {
      handleSseEvent(message.event as string, message.data);
      scheduleRefresh();
    }
    if (message.type === 'syncUpdate') {
      if (Array.isArray(message.downloads)) setActiveDownloads(message.downloads as DownloadStatus[]);
      if (Array.isArray(message.history)) setHistoryDownloads(message.history as HistoryEntry[]);
    }
    if (message.type === 'serverStatus') {
      if (typeof message.connected === 'boolean') setServerConnected(message.connected);
    }
  }

  onMount(async () => {
    await loadSettings();
    await fetchDownloads();

    pollInterval = setInterval(() => fetchDownloads(false), 15000);
    healthInterval = setInterval(async () => {
      try {
        const res = await browser.runtime.sendMessage({ type: 'checkHealth' }) as { healthy?: boolean };
        if (res && typeof res.healthy === 'boolean') setServerConnected(res.healthy);
      } catch {
        setServerConnected(false);
      }
    }, 3000);

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
