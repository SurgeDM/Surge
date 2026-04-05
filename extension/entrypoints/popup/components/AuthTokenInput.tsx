import { authToken, setAuthToken, authValid } from '../store';
import { createSignal } from 'solid-js';

export default function AuthTokenInput() {
  const [status, setStatus] = createSignal('');

  const handleSave = async () => {
    const currentToken = authToken();
    const token = currentToken.trim();

    if (!token) {
      setStatus('Connect to Surge first');
      return;
    }

    if (authValid()) {
      // Delete token
      try {
        await browser.runtime.sendMessage({ type: 'setAuthToken', token: '' });
        await browser.runtime.sendMessage({ type: 'setAuthVerified', verified: false });
        setAuthToken('');
        setStatus('');
      } catch {
        setStatus('Failed to delete token');
      }
      return;
    }

    setStatus('Validating...');
    try {
      const saveResult = await browser.runtime.sendMessage({ type: 'setAuthToken', token }) as { success?: boolean; error?: string };
      if (!saveResult?.success) throw new Error(saveResult?.error || 'Failed to save');

      const result = await browser.runtime.sendMessage({ type: 'validateAuth' }) as { ok?: boolean; error?: string };
      if (result?.ok) {
        setStatus('Token valid');
        await browser.runtime.sendMessage({ type: 'setAuthVerified', verified: true });
      } else {
        setStatus(result?.error === 'no_server' ? 'Connect to Surge first' : 'Token invalid');
        await browser.runtime.sendMessage({ type: 'setAuthVerified', verified: false });
      }
    } catch {
      setStatus('Validation failed');
    }
  };

  return (
    <div class="auth-row">
      <div class="auth-label">
        <label for="authToken">Auth Token</label>
        <span class="auth-help" data-tooltip="Get the token by running: surge token">?</span>
      </div>
      <div class="auth-input">
        <input
          type="password"
          id="authToken"
          value={authToken()}
          placeholder="Paste token from Surge"
          onInput={(e) => setAuthToken((e.target as HTMLInputElement).value)}
        />
        <button id="saveToken" onClick={handleSave}>
          {authValid() ? 'Delete' : 'Save'}
        </button>
      </div>
      <div class={`auth-status${status() === 'Token valid' ? ' ok' : status() ? ' err' : ''}`}>{status()}</div>
    </div>
  );
}
