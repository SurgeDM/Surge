import type { DownloadStatus } from '../store/types';
import { currentView, setCurrentView, historyDownloads } from '../store';
import DownloadItem from './DownloadItem';
import ViewSwitch from './ViewSwitch';
import SettingsView from './SettingsView';

interface Props {
  activeDownloads: DownloadStatus[];
}

export default function DownloadList(props: Props) {
  const getVisibleDownloads = (): DownloadStatus[] => {
    const view = currentView();
    if (view === 'active') return props.activeDownloads.filter(dl => dl.status !== 'completed');
    return historyDownloads() as DownloadStatus[];
  };

  const sortDownloads = (items: DownloadStatus[]): DownloadStatus[] => {
    const order = { downloading: 0, paused: 1, queued: 2, completed: 3, error: 4 };
    return [...items].sort((a, b) => {
      const diff = (order[a.status] ?? 5) - (order[b.status] ?? 5);
      return diff !== 0 ? diff : (b.added_at || 0) - (a.added_at || 0);
    });
  };

  const getEmptyMessage = (): { title: string; hint: string } => {
    if (currentView() === 'history') {
      return { title: 'No history downloads', hint: 'Completed downloads will appear here' };
    }
    return { title: 'No active downloads', hint: 'Downloads will appear here automatically' };
  };

  return (
    <div class="downloads-list" id="downloadsList">
      <div class="downloads-list-header">
        <ViewSwitch currentView={currentView()} onChange={setCurrentView} />
      </div>
      <div class="downloads-list-content">
        {currentView() === 'settings'
          ? <SettingsView />
          : sortDownloads(getVisibleDownloads()).map(dl => <DownloadItem download={dl} />)
        }

        {currentView() !== 'settings' && sortDownloads(getVisibleDownloads()).length === 0 && (() => {
          const msg = getEmptyMessage();
          return (
            <div class="empty-state" id="emptyState">
              <div class="empty-icon">&#x1F4E6;</div>
              <p>{msg.title}</p>
              <p class="empty-hint">{msg.hint}</p>
            </div>
          );
        })()}
      </div>
    </div>
  );
}
