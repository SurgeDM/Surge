export default function StatusBadge(props: { connected: boolean }) {
  return (
    <div class={`status-badge ${props.connected ? 'online' : 'offline'}`}>
      <span class={`status-dot ${props.connected ? 'online' : 'offline'}`} />
      <span class="status-text">{props.connected ? 'Connected' : 'Offline'}</span>
    </div>
  );
}
