import type { ViewMode } from '../store';

export default function ViewSwitch(props: {
  currentView: ViewMode;
  onChange: (view: ViewMode) => void;
}) {
  return (
    <div class="view-switch" id="viewSwitch">
      <button
        class={`view-tab${props.currentView === 'active' ? ' active' : ''}`}
        type="button"
        onClick={() => props.onChange('active')}
      >
        Active
      </button>
      <button
        class={`view-tab${props.currentView === 'history' ? ' active' : ''}`}
        type="button"
        onClick={() => props.onChange('history')}
      >
        History
      </button>
    </div>
  );
}
