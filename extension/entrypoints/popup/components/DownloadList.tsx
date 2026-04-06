import type { DownloadStatus } from '../store/types';
import { createMemo } from 'solid-js';
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
    added_at: entry.completed_at,
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

export default function DownloadList(props: Props) {
  const visibleDownloads = createMemo<DownloadStatus[]>(() => {
    const view = currentView();
    if (view === 'active') {
      return props.activeDownloads.filter((download) => download.status !== 'completed');
    }

    return historyDownloads().map(mapHistoryEntryToDownload);
  });

  const sortedDownloads = createMemo(() => sortDownloads(visibleDownloads()));
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
          : sortedDownloads().map((download) => <DownloadItem download={download} />)
        }

        {currentView() !== 'settings' && sortedDownloads().length === 0 && (
          <div class="empty-state" id="emptyState">
            <div class="empty-icon">&#x1F4E6;</div>
            <p>{emptyMessage().title}</p>
            <p class="empty-hint">{emptyMessage().hint}</p>
          </div>
        )}
      </div>
    </div>
  );
}
