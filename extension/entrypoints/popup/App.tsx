import { onMount, onCleanup } from 'solid-js';
import {
  serverConnected, setServerConnected,
  activeDownloads, setActiveDownloads,
  serverUrl, setServerUrl,
  authToken, setAuthToken,
  authValid, setAuthValid,
  currentView, setCurrentView,
  setHistoryDownloads,
  interceptEnabled, setInterceptEnabled,
  handleSseEvent,
  ViewMode,
} from './store';
import StatusBadge from './components/StatusBadge';
import DownloadList from './components/DownloadList';
import DuplicateModal from './components/DuplicateModal';
import ServerUrlInput from './components/ServerUrlInput';
import AuthTokenInput from './components/AuthTokenInput';
import ViewSwitch from './components/ViewSwitch';
import { normalizeToken } from './lib/utils';
import './popup.css';

export default function App() {
  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let healthInterval: ReturnType<typeof setInterval> | null = null;

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

  async function fetchDownloads(full = false) {
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

  function handleViewChange(view: ViewMode) {
    setCurrentView(view);
    if (view === 'history') {
      fetchHistory();
    }
  }

  function onMessageListener(message: any) {
    if (message.type === 'sseEvent') {
      handleSseEvent(message.event, message.data);
      // Refresh full list periodically after SSE events to ensure consistency
      setTimeout(() => fetchDownloads(false), 2000);
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
    browser.runtime.onMessage.removeListener(onMessageListener);
  });

  return (
    <div class="container">
      <header class="header">
        <div class="logo">
          <img src="/icons/icon48.png" alt="Surge" />
          <h1>SURGE</h1>
        </div>
        <StatusBadge connected={serverConnected()} />
      </header>

      <section class="downloads-section">
        <div class="downloads-header">
          <div class="downloads-title-row">
            <span class="section-title">Downloads</span>
            <span class="download-count">{activeDownloads().length}</span>
          </div>
          <ViewSwitch currentView={currentView()} onChange={handleViewChange} />
        </div>

        <DownloadList
          view={currentView()}
          activeDownloads={activeDownloads()}
        />
      </section>

      <footer class="footer">
        <ServerUrlInput />
        <AuthTokenInput />
        <div class="toggle-row">
          <span>Intercept Downloads</span>
          <div class="toggle">
            <input
              type="checkbox"
              checked={interceptEnabled()}
              onChange={async (e) => {
                const checked = (e.target as HTMLInputElement).checked;
                setInterceptEnabled(checked);
                await browser.runtime.sendMessage({ type: 'setStatus', enabled: checked });
              }}
            />
            <span class="toggle-slider" />
          </div>
        </div>
      </footer>

      <DuplicateModal />
    </div>
  );
}
