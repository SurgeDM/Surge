export default function StatusBadge(props: { connected: boolean; authValid: boolean; onClick?: () => void }) {
  const status = () => {
    if (!props.connected) return 'offline';
    if (!props.authValid) return 'warning';
    return 'online';
  };

  const text = () => {
    if (!props.connected) return 'Offline';
    if (!props.authValid) return 'Invalid';
    return 'Connected';
  };

  return (
    <button
      type="button"
      class={`status-badge ${status()}`}
      onClick={() => props.onClick?.()}
      aria-label={`Server status: ${text()}`}
    >
      <span class={`status-dot ${status()}`} aria-hidden="true" />
      <span class="status-text">{text()}</span>
    </button>
  );
}
