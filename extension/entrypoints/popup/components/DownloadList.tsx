import type { DownloadStatus } from '../store/types';
import { createMemo, For } from 'solid-js';
import { currentView, setCurrentView, historyDownloads } from '../store';
import type { ViewMode } from '../store';
import DownloadItem from './DownloadItem';
import ViewSwitch from './ViewSwitch';
import SettingsView from './SettingsView';

interface Props {
  activeDownloads: DownloadStatus[];
  onViewChange?: (view: ViewMode) => void;
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
      <svg viewBox="0 0 120 120" class="empty-illustration">
        <defs>
          <linearGradient id="empty-wireframe" x1="24" y1="24" x2="96" y2="92" gradientUnits="userSpaceOnUse">
            <stop offset="0%" stop-color="#ff79c6" />
            <stop offset="100%" stop-color="#8be9fd" />
          </linearGradient>
        </defs>
        <path class="empty-illustration-frame" d="M60 18 92 36v48L60 102 28 84V36l32-18Z" />
        <path class="empty-illustration-frame" d="M28 36 60 55l32-19" />
        <path class="empty-illustration-frame" d="M60 55v47" />
        <path class="empty-illustration-detail" d="M40 73h10l7-12 11 20 7-11h6" />
      </svg>
    </div>
  );
}

export default function DownloadList(props: Props) {
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
      </div>
      <div class="downloads-list-content">
        {currentView() === 'settings'
          ? <SettingsView />
          : currentView() === 'active'
            ? <For each={sortedActiveDownloadIds()}>{(id) => <DownloadItem download={() => activeDownloadById().get(id)!} />}</For>
            : <For each={sortedHistoryDownloads()}>{(download) => <DownloadItem download={() => download} />}</For>
        }

        {currentView() === 'active' && sortedActiveDownloadIds().length === 0 && (
          <div class="empty-state" id="emptyState">
            <EmptyStateGraphic />
            <h2 class="empty-title">{emptyMessage().title}</h2>
            <p class="empty-hint">{emptyMessage().hint}</p>
          </div>
        )}

        {currentView() === 'history' && sortedHistoryDownloads().length === 0 && (
          <div class="empty-state" id="emptyState">
            <EmptyStateGraphic />
            <h2 class="empty-title">{emptyMessage().title}</h2>
            <p class="empty-hint">{emptyMessage().hint}</p>
          </div>
        )}
      </div>
    </div>
  );
}
