import { createSignal } from 'solid-js';
import type { DownloadStatus } from '../store/types';
import DownloadItem from './DownloadItem';
import ViewSwitch from './ViewSwitch';

interface Props {
  activeDownloads: DownloadStatus[];
}

export default function DownloadList(props: Props) {
  const [view, setView] = createSignal<'active' | 'history'>('active');

  const getVisibleDownloads = (): DownloadStatus[] => {
    return view() === 'active'
      ? props.activeDownloads.filter(dl => dl.status !== 'completed')
      : props.activeDownloads;
  };

  const sortDownloads = (items: DownloadStatus[]): DownloadStatus[] => {
    const statusOrder: Record<string, number> = { downloading: 0, paused: 1, queued: 2, completed: 3, error: 4 };
    return [...items].sort((left, right) => {
      const orderLeft = statusOrder[left.status] ?? 5;
      const orderRight = statusOrder[right.status] ?? 5;
      if (orderLeft !== orderRight) return orderLeft - orderRight;
      return (right.added_at || 0) - (left.added_at || 0);
    });
  };

  const getEmptyMessage = (): { title: string; hint: string } => {
    if (view() === 'history') {
      return { title: 'No history downloads', hint: 'Completed downloads will appear here' };
    }
    return { title: 'No active downloads', hint: 'Downloads will appear here automatically' };
  };

  return (
    <div class="downloads-list" id="downloadsList">
      <div class="downloads-list-header">
        <ViewSwitch currentView={view()} onChange={setView} />
      </div>
      {(() => {
        const visible = sortDownloads(getVisibleDownloads());
        if (visible.length === 0) {
          const msg = getEmptyMessage();
          return (
            <div class="empty-state" id="emptyState">
              <div class="empty-icon">&#x1F4E6;</div>
              <p>{msg.title}</p>
              <p class="empty-hint">{msg.hint}</p>
            </div>
          );
        }
        return visible.map(dl => (
          <DownloadItem download={dl} />
        ));
      })()}
    </div>
  );
}
