import type { DownloadStatus } from '../store/types';
import { createMemo, createSignal, For } from 'solid-js';
import { currentView, setCurrentView, historyDownloads, serverConnected } from '../store';
import type { ViewMode } from '../store';
import DownloadItem from './DownloadItem';
import ViewSwitch from './ViewSwitch';
import SettingsView from './SettingsView';

interface Props {
  activeDownloads: DownloadStatus[];
  onViewChange?: (view: ViewMode) => void;
  onRefresh?: () => void;
  refreshing?: boolean;
}

const STATUS_ORDER: Record<DownloadStatus['status'], number> = {
  downloading: 0,
  paused: 1,
  queued: 2,
  completed: 3,
  error: 4,
};

function normalizeStatus(status: string): DownloadStatus['status'] {
  if (status === 'downloading' || status === 'paused' || status === 'queued' || status === 'error') {
    return status;
  }

  return 'completed';
}

function mapHistoryEntryToDownload(entry: ReturnType<typeof historyDownloads>[number]): DownloadStatus {
  return {
    ...entry,
    status: normalizeStatus(entry.status),
    progress: entry.total_size > 0 ? (entry.downloaded / entry.total_size) * 100 : 100,
    speed: 0,
    eta: 0,
    connections: 0,
    added_at: entry.completed_at * 1000,
    error: undefined,
  };
}

function sortDownloads(downloads: DownloadStatus[]): DownloadStatus[] {
  return [...downloads].sort((left, right) => {
    const orderDifference = STATUS_ORDER[left.status] - STATUS_ORDER[right.status];
    if (orderDifference !== 0) return orderDifference;
    return (right.added_at || 0) - (left.added_at || 0);
  });
}

function EmptyStateGraphic() {
  return (
    <div class="empty-icon" aria-hidden="true">
      <svg viewBox="0 0 48 48" class="empty-illustration" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <path d="M24 10v16" />
        <path d="M18 20l6 6 6-6" />
        <path d="M12 30h7l3 4h4l3-4h7" />
        <path d="M12 30v4a4 4 0 0 0 4 4h16a4 4 0 0 0 4-4v-4" />
      </svg>
    </div>
  );
}

export default function DownloadList(props: Props) {
  const [clearingCompleted, setClearingCompleted] = createSignal(false);
  const [clearingFailed, setClearingFailed] = createSignal(false);

  const handleClearCompleted = async () => {
    if (clearingCompleted() || !serverConnected()) return;
    setClearingCompleted(true);
    try {
      await browser.runtime.sendMessage({ type: 'clearCompleted' });
      props.onRefresh?.();
    } finally {
      setClearingCompleted(false);
    }
  };

  const handleClearFailed = async () => {
    if (clearingFailed() || !serverConnected()) return;
    setClearingFailed(true);
    try {
      await browser.runtime.sendMessage({ type: 'clearFailed' });
      props.onRefresh?.();
    } finally {
      setClearingFailed(false);
    }
  };

  const activeDownloads = createMemo<DownloadStatus[]>(() =>
    props.activeDownloads.filter((download) => download.status !== 'completed'),
  );
  const activeDownloadById = createMemo(() =>
    new Map(activeDownloads().map((download) => [download.id, download] as const)),
  );
  const sortedActiveDownloadIds = createMemo(() =>
    sortDownloads(activeDownloads()).map((download) => download.id),
  );
  const sortedHistoryDownloads = createMemo(() =>
    sortDownloads(historyDownloads().map(mapHistoryEntryToDownload)),
  );
  const emptyMessage = createMemo(() => {
    if (currentView() === 'history') {
      return { title: 'No history downloads', hint: 'Completed downloads will appear here' };
    }

    return { title: 'No active downloads', hint: 'Downloads will appear here automatically' };
  });

  return (
    <div class="downloads-list" id="downloadsList">
      <div class="downloads-list-header">
        <ViewSwitch currentView={currentView()} onChange={props.onViewChange || setCurrentView} />
        {currentView() === 'history' && (
          <>
            <button
              type="button"
              class={`clear-btn completed${clearingCompleted() ? ' clearing' : ''}`}
              title="Clear completed downloads"
              aria-label="Clear completed downloads"
              disabled={clearingCompleted() || !serverConnected()}
              onClick={() => { void handleClearCompleted(); }}
            >
              <svg viewBox="0 0 24 24" class="clear-icon" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </button>
            <button
              type="button"
              class={`clear-btn failed${clearingFailed() ? ' clearing' : ''}`}
              title="Clear failed downloads"
              aria-label="Clear failed downloads"
              disabled={clearingFailed() || !serverConnected()}
              onClick={() => { void handleClearFailed(); }}
            >
              <svg viewBox="0 0 24 24" class="clear-icon" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </>
        )}
        <button
          type="button"
          class={`refresh-btn${props.refreshing ? ' refreshing' : ''}`}
          title="Refresh"
          aria-label="Refresh"
          disabled={props.refreshing}
          onClick={() => props.onRefresh?.()}
        >
          <svg viewBox="0 0 24 24" class="refresh-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M21 12a9 9 0 0 1-15.5 6.2" />
            <path d="M3 12A9 9 0 0 1 18.5 5.8" />
            <path d="M18 2v4h4" />
            <path d="M6 22v-4H2" />
          </svg>
        </button>
      </div>
      <div class="downloads-list-content">
        {currentView() === 'settings'
          ? <SettingsView />
          : currentView() === 'active'
            ? <For each={sortedActiveDownloadIds()}>{(id) => <DownloadItem download={() => activeDownloadById().get(id)!} onActionComplete={props.onRefresh} />}</For>
            : <For each={sortedHistoryDownloads()}>{(download) => <DownloadItem download={() => download} onActionComplete={props.onRefresh} />}</For>
        }

        {currentView() === 'active' && sortedActiveDownloadIds().length === 0 && (
          <div class="empty-state" id="emptyState-active">
            <EmptyStateGraphic />
            <h2 class="empty-title">{emptyMessage().title}</h2>
            <p class="empty-hint">{emptyMessage().hint}</p>
          </div>
        )}

        {currentView() === 'history' && sortedHistoryDownloads().length === 0 && (
          <div class="empty-state" id="emptyState-history">
            <EmptyStateGraphic />
            <h2 class="empty-title">{emptyMessage().title}</h2>
            <p class="empty-hint">{emptyMessage().hint}</p>
          </div>
        )}
      </div>
    </div>
  );
}
