import type { ViewMode } from '../store';

export default function ViewSwitch(props: {
  currentView: ViewMode;
  onChange: (view: ViewMode) => void;
}) {
  return (
    <div class="view-switch" id="viewSwitch" role="tablist">
      <button
        class={`view-tab${props.currentView === 'active' ? ' active' : ''}`}
        type="button"
        role="tab"
        aria-selected={props.currentView === 'active'}
        onClick={() => props.onChange('active')}
      >
        Active
      </button>
      <button
        class={`view-tab${props.currentView === 'history' ? ' active' : ''}`}
        type="button"
        role="tab"
        aria-selected={props.currentView === 'history'}
        onClick={() => props.onChange('history')}
      >
        History
      </button>
      <button
        class={`view-tab${props.currentView === 'settings' ? ' active' : ''}`}
        type="button"
        role="tab"
        aria-selected={props.currentView === 'settings'}
        aria-label="Settings"
        onClick={() => props.onChange('settings')}
      >
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
        </svg>
      </button>
    </div>
  );
}
