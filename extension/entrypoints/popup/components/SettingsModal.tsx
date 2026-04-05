import { createSignal, onMount, onCleanup } from 'solid-js';
import {
  serverUrl, setServerUrl,
  authToken, setAuthToken,
  setAuthValid,
  interceptEnabled, setInterceptEnabled,
} from '../store';
import { normalizeToken, normalizeServerUrl } from '../lib/utils';
import type { Theme } from '../store/types';

interface Props {
  isOpen: () => boolean;
  onClose: () => void;
}

export default function SettingsModal(props: Props) {
  const [serverStatus, setServerStatus] = createSignal('');
  const [tokenStatus, setTokenStatus] = createSignal('');
  const [tokenFocused, setTokenFocused] = createSignal(false);

  const handleServerSave = async () => {
    const url = normalizeServerUrl(serverUrl());
    setServerUrl(url);
    setServerStatus('Saving...');
    try {
      await browser.runtime.sendMessage({ type: 'setServerUrl', url });
      setServerStatus('Saved');
      setTimeout(() => setServerStatus(''), 2000);
    } catch {
      setServerStatus('Failed to save');
    }
  };

  const handleSaveToken = async () => {
    const token = authToken();
    const normalized = normalizeToken(token);
    setAuthToken(normalized);
    setTokenStatus('Saving...');
    try {
      await browser.runtime.sendMessage({ type: 'setAuthToken', token: normalized });
      setTokenStatus('Saved');
      setTimeout(() => setTokenStatus(''), 2000);
      try {
        const res = await browser.runtime.sendMessage({ type: 'validateAuth' }) as { ok?: boolean };
        if (res?.ok) {
          setAuthValid(true);
          await browser.runtime.sendMessage({ type: 'setAuthVerified', verified: true });
        } else {
          setAuthValid(false);
        }
      } catch {
        setAuthValid(false);
      }
    } catch {
      setTokenStatus('Failed to save');
    }
  };

  const handleInterceptToggle = async (checked: boolean) => {
    setInterceptEnabled(checked);
    await browser.runtime.sendMessage({ type: 'setStatus', enabled: checked });
  };

  const [selectedTheme, setSelectedTheme] = createSignal<Theme>('dark');

  const handleThemeChange = (theme: Theme) => {
    setSelectedTheme(theme);
  };

  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && props.isOpen()) props.onClose();
  };

  onMount(() => {
    document.addEventListener('keydown', onKey);
  });

  onCleanup(() => {
    document.removeEventListener('keydown', onKey);
  });

  return (
    <div class={`modal-overlay${props.isOpen() ? '' : ' hidden'}`} onClick={(e) => {
      if (e.target === e.currentTarget) props.onClose();
    }}>
      <div class="settings-container">
        <div class="settings-header">
          <h2 class="settings-title">Settings</h2>
          <button class="settings-close" onClick={props.onClose}>&#x2715;</button>
        </div>

        {/* Intercept (top priority) */}
        <div class="settings-group">
          <h3 class="settings-group-title">General</h3>
          <div class="toggle-row">
            <span>Intercept Downloads</span>
            <div class="toggle">
              <input
                type="checkbox"
                checked={interceptEnabled()}
                onChange={(e) => { void handleInterceptToggle((e.target as HTMLInputElement).checked); }}
              />
              <span class="toggle-slider" />
            </div>
          </div>
        </div>

        {/* Server */}
        <div class="settings-group">
          <h3 class="settings-group-title">Server</h3>
          <div class="settings-field">
            <div class="settings-field-row">
              <label class="settings-label">Server URL</label>
              {serverStatus() && (
                <span class={`auth-status${serverStatus() === 'Saved' ? ' ok' : ' err'}`}>{serverStatus()}</span>
              )}
            </div>
            <div class="auth-input">
              <input
                type="text"
                value={serverUrl()}
                placeholder="http://127.0.0.1:1700"
                onInput={(e) => { setServerUrl((e.target as HTMLInputElement).value); }}
              />
              <button onClick={handleServerSave}>Save</button>
            </div>
          </div>

          <div class="settings-field">
            <div class="settings-field-row">
              <label class="settings-label">Auth Token</label>
              {tokenStatus() && !tokenFocused() && (
                <span class={`auth-status${tokenStatus() === 'Saved' ? ' ok' : ' err'}`}>{tokenStatus()}</span>
              )}
            </div>
            <div class="auth-input">
              <input
                type="password"
                value={authToken()}
                placeholder="Enter your token"
                onInput={(e) => {
                  setAuthToken((e.target as HTMLInputElement).value);
                  setTokenStatus('');
                }}
                onFocus={() => setTokenFocused(true)}
                onBlur={() => setTokenFocused(false)}
              />
              <button onClick={handleSaveToken}>Save</button>
            </div>
          </div>
        </div>

        {/* Appearance */}
        <div class="settings-group">
          <h3 class="settings-group-title">Appearance</h3>
          <div class="theme-options">
            {(['dark', 'light', 'auto'] as Theme[]).map(t => (
              <button
                class={`theme-btn${selectedTheme() === t ? ' active' : ''}`}
                onClick={() => handleThemeChange(t)}
              >
                {t === 'dark' ? '\u{1F319} Dark' : t === 'light' ? '\u{2600}\u{FE0F} Light' : '\u{1F4BB} Auto'}
              </button>
            ))}
          </div>
        </div>

        {/* GitHub link */}
        <a
          href="https://github.com/SurgeDM/Surge"
          target="_blank"
          rel="noopener noreferrer"
          class="github-link"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
          </svg>
          SurgeDM/Surge
        </a>
      </div>
    </div>
  );
}
