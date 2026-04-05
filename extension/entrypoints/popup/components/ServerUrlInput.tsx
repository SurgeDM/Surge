import { serverUrl, setServerUrl } from '../store';
import { createSignal } from 'solid-js';

export default function ServerUrlInput() {
  const [status, setStatus] = createSignal('');
  let inputRef: HTMLInputElement | undefined;

  const handleSave = async () => {
    let url = inputRef?.value.trim() || '';
    if (url && !url.startsWith('http://') && !url.startsWith('https://')) {
      url = 'http://' + url;
    }

    setServerUrl(url);
    setStatus('Saving...');

    try {
      await browser.runtime.sendMessage({ type: 'setServerUrl', url });
      setStatus('Saved');
      setTimeout(() => setStatus(''), 3000);
    } catch {
      setStatus('Failed to save');
    }
  };

  return (
    <div class="auth-row">
      <div class="auth-label">
        <label for="serverUrl">Server URL</label>
      </div>
      <div class="auth-input">
        <input
          type="text"
          id="serverUrl"
          ref={inputRef}
          value={serverUrl()}
          placeholder="http://127.0.0.1:1700"
          onInput={() => setStatus('')}
        />
        <button id="saveServerUrl" onClick={handleSave}>Save</button>
      </div>
      <div class={`auth-status${status() === 'Saved' ? ' ok' : status() ? ' err' : ''}`}>{status()}</div>
    </div>
  );
}
