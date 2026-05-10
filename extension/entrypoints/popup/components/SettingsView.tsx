import { createSignal } from 'solid-js';
import { STORAGE_KEYS } from '../../../lib/storage';
import {
  serverUrl, setServerUrl,
  serverUrlLocked, setServerUrlLocked,
  serverConnected,
  authToken, setAuthToken,
  authTokenLocked, setAuthTokenLocked,
  authValid, setAuthValid,
  interceptEnabled, setInterceptEnabled,
  notificationsEnabled, setNotificationsEnabled,
} from '../store';
import { normalizeToken, normalizeServerUrl } from '../lib/utils';

function saveStatusSignal() {
  const [status, setStatus] = createSignal('');
  let timer: ReturnType<typeof setTimeout> | null = null;
  const show = (msg: string, ms = 2000) => {
    if (timer) clearTimeout(timer);
    setStatus(msg);
    if (ms > 0) timer = setTimeout(() => setStatus(''), ms);
  };
  return [status, show] as const;
}

export default function SettingsView() {
  const [serverStatus, showServerStatus] = saveStatusSignal();
  const [tokenStatus, showTokenStatus] = saveStatusSignal();
  const [tokenFocused, setTokenFocused] = createSignal(false);
  const extensionVersion = browser.runtime.getManifest().version;
  const isFirefox = (browser.runtime.getURL as (path?: string) => string)('').startsWith('moz-extension:');

  const handleServerSave = async () => {
    const url = normalizeServerUrl(serverUrl());
    setServerUrl(url);
    showServerStatus('Saving...');
    try {
      await browser.storage.local.set({ [STORAGE_KEYS.SERVER_URL]: url });
      setServerUrlLocked(url.length > 0);
      showServerStatus('Saved');
    } catch {
      showServerStatus('Failed to save');
    }
  };

  const handleServerDelete = async () => {
    setServerUrl('');
    setServerUrlLocked(false);
    showServerStatus('Removing...');
    try {
      await browser.storage.local.set({ [STORAGE_KEYS.SERVER_URL]: '' });
      showServerStatus('Removed');
    } catch {
      setServerUrlLocked(true);
      showServerStatus('Failed to remove');
    }
  };

  const handleSaveToken = async () => {
    const token = normalizeToken(authToken());
    setAuthToken(token);
    showTokenStatus('Saving...');
    try {
      await browser.storage.local.set({
        [STORAGE_KEYS.TOKEN]: token,
        [STORAGE_KEYS.VERIFIED]: 'false',
      });
      setAuthTokenLocked(token.length > 0);
      showTokenStatus('Saved');
      const res = await browser.runtime.sendMessage({ type: 'validateAuth', token }).catch(() => null) as { ok?: boolean; error?: string } | null;
      setAuthValid(res?.ok ?? false);
      if (res?.ok) {
        await browser.storage.local.set({ [STORAGE_KEYS.VERIFIED]: 'true' });
      } else if (res?.error === 'no_server') {
        showTokenStatus('Server unavailable');
      } else if (res?.error === 'invalid_token') {
        showTokenStatus('Invalid Token');
      }
    } catch {
      showTokenStatus('Failed to save');
    }
  };

  const handleDeleteToken = async () => {
    setAuthToken('');
    setAuthTokenLocked(false);
    setAuthValid(false);
    showTokenStatus('Removing...');
    try {
      await browser.storage.local.set({
        [STORAGE_KEYS.TOKEN]: '',
        [STORAGE_KEYS.VERIFIED]: 'false',
      });
      showTokenStatus('Removed');
    } catch {
      setAuthTokenLocked(true);
      showTokenStatus('Failed to remove');
    }
  };

  const handleInterceptToggle = async (checked: boolean) => {
    setInterceptEnabled(checked);
    await browser.storage.local.set({ [STORAGE_KEYS.INTERCEPT]: checked });
  };
  const handleNotificationsToggle = async (checked: boolean) => {
    setNotificationsEnabled(checked);
    await browser.storage.local.set({ [STORAGE_KEYS.NOTIFICATIONS]: checked });
  };

  return (
    <div>
      <div class="settings-group">
        <label class="toggle-row">
          <span>Intercept Downloads</span>
          <div class="toggle">
            <input
              type="checkbox"
              checked={interceptEnabled()}
              onChange={(e) => { void handleInterceptToggle((e.target as HTMLInputElement).checked); }}
            />
            <span class="toggle-slider" />
          </div>
        </label>
        <label class="toggle-row">
          <span>Show Notifications</span>
          <div class="toggle">
            <input
              type="checkbox"
              checked={notificationsEnabled()}
              onChange={(e) => { void handleNotificationsToggle((e.target as HTMLInputElement).checked); }}
            />
            <span class="toggle-slider" />
          </div>
        </label>
      </div>

      <div class="settings-group">
        <h3 class="settings-group-title">Server</h3>
        <div class="settings-field">
          <label class="settings-label" for="server-url">Server URL</label>
          <div class="auth-input settings-input-row">
            <input
              id="server-url"
              type="text"
              value={serverUrl()}
              placeholder="http://127.0.0.1:1700"
              disabled={serverUrlLocked()}
              onInput={(e) => { setServerUrl((e.target as HTMLInputElement).value); }}
            />
            <button onClick={() => { void (serverUrlLocked() ? handleServerDelete() : handleServerSave()); }}>
              {serverUrlLocked() ? 'Delete' : 'Save'}
            </button>
          </div>
          {serverStatus() && (
            <div class={`auth-status below${serverStatus() === 'Saved' || serverStatus() === 'Removed' ? ' ok' : serverStatus().endsWith('...') ? '' : ' err'}`}>{serverStatus()}</div>
          )}
        </div>

        <div class="settings-field">
          <label class="settings-label" for="auth-token">Auth Token</label>
          <div class="auth-input settings-input-row">
            <input
              id="auth-token"
              type="password"
              value={authToken()}
              placeholder="Enter your token"
              disabled={authTokenLocked()}
              onInput={(e) => {
                setAuthToken((e.target as HTMLInputElement).value);
                showTokenStatus('');
              }}
              onFocus={() => setTokenFocused(true)}
              onBlur={() => setTokenFocused(false)}
            />
            <button onClick={() => { void (authTokenLocked() ? handleDeleteToken() : handleSaveToken()); }}>
              {authTokenLocked() ? 'Delete' : 'Save'}
            </button>
          </div>
          <div class="settings-help">
            Token can be obtained from <strong>TUI &gt; Settings &gt; Extension</strong>
          </div>
          {tokenStatus() && !tokenFocused() && (
            <div class={`auth-status below${tokenStatus() === 'Saved' || tokenStatus() === 'Removed' ? ' ok' : tokenStatus().endsWith('...') ? '' : ' err'}`}>{tokenStatus()}</div>
          )}
          {authTokenLocked() && !authValid() && serverConnected() && !tokenFocused() && !tokenStatus() && (
            <div class="auth-status below err">Invalid Token</div>
          )}
          {authTokenLocked() && !authValid() && !serverConnected() && !tokenFocused() && !tokenStatus() && (
            <div class="auth-status below err">Server unavailable</div>
          )}
          {!authToken() && !tokenFocused() && !tokenStatus() && (
            <div class="auth-status below err">Token is Required</div>
          )}
        </div>
      </div>

      <div class="settings-group">
        <div class="settings-group-header">
          <h3 class="settings-group-title">Support</h3>
          <div class="version-badge">v{extensionVersion}</div>
        </div>
        <a
          href="https://github.com/SurgeDM/Surge"
          target="_blank"
          rel="noopener noreferrer"
          class="support-link"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
          </svg>
          SurgeDM/Surge
        </a>
        <a
          href="https://github.com/SurgeDM/Surge/issues/new?template=extension_bug_report.md"
          target="_blank"
          rel="noopener noreferrer"
          class="support-link"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="m8 2 1.88 1.88" />
            <path d="M14.12 3.88 16 2" />
            <path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1" />
            <path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6" />
            <path d="M12 20v-9" />
            <path d="M6.53 9C4.6 8.8 3 7.1 3 5" />
            <path d="M6 13H2" />
            <path d="M3 21c0-2.1 1.7-3.9 3.8-4" />
            <path d="M20.97 5c0 2.1-1.6 3.8-3.5 4" />
            <path d="M22 13h-4" />
            <path d="M17.2 17c2.1.1 3.8 1.9 3.8 4" />
          </svg>
          Report a Bug
        </a>
        {isFirefox && (
          <a
            href="https://addons.mozilla.org/en-US/firefox/addon/surge"
            target="_blank"
            rel="noopener noreferrer"
            class="support-link"
          >
            Firefox Add-ons
          </a>
        )}
      </div>
    </div>
  );
}
