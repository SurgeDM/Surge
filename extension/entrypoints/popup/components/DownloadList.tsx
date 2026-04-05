import type { DownloadStatus } from '../store/types';
import DownloadItem from './DownloadItem';

interface Props {
  view: 'active' | 'history';
  activeDownloads: DownloadStatus[];
}

export default function DownloadList(props: Props) {
  const getVisibleDownloads = (): DownloadStatus[] => {
    const items = [...props.activeDownloads.values()];
    return props.view === 'active'
      ? items.filter(dl => dl.status !== 'completed')
      : items;
  };

  const sortDownloads = (items: DownloadStatus[]): DownloadStatus[] => {
    const statusOrder: Record<string, number> = { downloading: 0, paused: 1, queued: 2, completed: 3, error: 4 };
    return items.sort((left, right) => {
      const orderLeft = statusOrder[left.status] ?? 5;
      const orderRight = statusOrder[right.status] ?? 5;
      if (orderLeft !== orderRight) return orderLeft - orderRight;
      return (right.added_at || 0) - (left.added_at || 0);
    });
  };

  const getEmptyMessage = (): { title: string; hint: string } => {
    if (props.view === 'history') {
      return { title: 'No history downloads', hint: 'Completed downloads will appear here' };
    }
    return { title: 'No active downloads', hint: 'Downloads will appear here automatically' };
  };

  return (
    <div class="downloads-list" id="downloadsList">
      {getVisibleDownloads().length === 0 ? (
        <div class="empty-state" id="emptyState">
          <div class="empty-icon">&#x1F4E6;</div>
          <p>{getEmptyMessage().title}</p>
          <p class="empty-hint">{getEmptyMessage().hint}</p>
        </div>
      ) : (
        sortDownloads(getVisibleDownloads()).map(dl => (
          <DownloadItem download={dl} />
        ))
      )}
    </div>
  );
}
