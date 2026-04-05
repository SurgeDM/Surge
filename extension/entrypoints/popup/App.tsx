import { createSignal, onMount, onCleanup } from 'solid-js';
import {
  serverConnected,
  setServerConnected,
  activeDownloads,
  setActiveDownloads,
  currentView,
  setHistoryDownloads,
  setInterceptEnabled,
  handleSseEvent,
} from './store';
import StatusBadge from './components/StatusBadge';
import DownloadList from './components/DownloadList';
import DuplicateModal from './components/DuplicateModal';
import SettingsModal from './components/SettingsModal';
import './popup.css';

export default function App() {
  const [showSettings, setShowSettings] = createSignal(false);
  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let healthInterval: ReturnType<typeof setInterval> | null = null;
  let refreshDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  function scheduleRefresh(): void {
    if (refreshDebounceTimer) return;
    refreshDebounceTimer = setTimeout(() => { refreshDebounceTimer = null; fetchDownloads(false); }, 2000);
  }

  async function loadSettings() {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getServerUrl' });
      if (res?.url !== undefined) setServerUrl(res.url);

      const tokenRes = await browser.runtime.sendMessage({ type: 'getAuthToken' });
      if (tokenRes?.token !== undefined) {
        setAuthToken(tokenRes.token);
        if (tokenRes.verified) setAuthValid(true);
      }

      const statusRes = await browser.runtime.sendMessage({ type: 'getStatus' });
      if (statusRes) setInterceptEnabled(statusRes.enabled !== false);
    } catch { /* ignore */ }
  }

  async function fetchDownloads(_full = false) {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getDownloads' });
      if (res?.downloads) {
        setServerConnected(res.connected);
        setActiveDownloads(res.downloads);
      }
      if (res?.authError) setAuthValid(false);

      if (currentView() === 'history' && res?.connected) {
        await fetchHistory();
      }
    } catch {
      setServerConnected(false);
    }
  }

  async function fetchHistory() {
    try {
      const res = await browser.runtime.sendMessage({ type: 'getHistory' });
      if (res?.history) {
        setHistoryDownloads(res.history);
      }
    } catch { /* ignore */ }
  }

  function onMessageListener(message: Record<string, unknown>) {
    if (message.type === 'sseEvent') {
      handleSseEvent(message.event, message.data);
      // Refresh full list periodically after SSE events to ensure consistency
      scheduleRefresh();
    }
    if (message.type === 'syncUpdate') {
      if (message.downloads) setActiveDownloads(message.downloads);
      if (message.history) setHistoryDownloads(message.history);
    }
    if (message.type === 'serverStatus') {
      setServerConnected(message.connected);
    }
  }

  onMount(async () => {
    await loadSettings();
    await fetchDownloads();

    pollInterval = setInterval(() => fetchDownloads(false), 1000);
    healthInterval = setInterval(async () => {
      try {
        const res = await browser.runtime.sendMessage({ type: 'checkHealth' });
        if (res && typeof res.healthy === 'boolean') setServerConnected(res.healthy);
      } catch {
        setServerConnected(false);
      }
    }, 3000);

    browser.runtime.onMessage.addListener(onMessageListener);
  });

  onCleanup(() => {
    if (pollInterval) clearInterval(pollInterval);
    if (healthInterval) clearInterval(healthInterval);
    if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
    browser.runtime.onMessage.removeListener(onMessageListener);
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
          <button class="settings-btn" onClick={() => setShowSettings(true)} title="Settings">
            &#x2699;
          </button>
        </div>
      </header>

      <section class="downloads-section">
        <DownloadList activeDownloads={activeDownloads()} />
      </section>

      <DuplicateModal />
      <SettingsModal isOpen={showSettings} onClose={() => setShowSettings(false)} />
    </div>
  );
}
