import { createSignal } from 'solid-js';
import type { JSX } from 'solid-js';
import { formatSpeed, formatETA, truncate, extractFilename, formatBytes } from '../lib/utils';
import type { DownloadStatus } from '../store/types';

interface Props {
  download: DownloadStatus;
}

export default function DownloadItem(props: Props) {
  const [expanded, setExpanded] = createSignal(false);
  const dl = () => props.download;

  const statusLabel = (): string => {
    switch (dl().status) {
      case 'downloading': return 'Downloading';
      case 'paused': return 'Paused';
      case 'queued': return 'Queued';
      case 'completed': return 'Completed';
      case 'error': return 'Error';
      default: return dl().status;
    }
  };

  const handleActionClick = async (e: MouseEvent) => {
    const btn = (e.target as HTMLElement).closest('.action-btn') as HTMLButtonElement | null;
    if (!btn) return;

    const action = btn.dataset.action;
    if (!action) return;

    btn.disabled = true;
    try {
      await browser.runtime.sendMessage({ type: action, id: dl().id });
    } finally {
      btn.disabled = false;
    }
  };

  const renderActionButtons = (): JSX.Element => {
    const s = dl().status;
    const buttons: JSX.Element[] = [];
    if (s === 'downloading') {
      buttons.push(<button class="action-btn pause" data-action="pauseDownload" title="Pause">&#x23F8;</button>);
    }
    if (s === 'paused' || s === 'queued') {
      buttons.push(<button class="action-btn resume" data-action="resumeDownload" title="Resume">&#x25B6;</button>);
    }
    if (s === 'completed') {
      buttons.push(<button class="action-btn open-folder" data-action="openFolder" title="Open folder">&#x1F4C1;</button>);
      buttons.push(<button class="action-btn open-file" data-action="openFile" title="Open file">&#x1F4C4;</button>);
    }
    if (s !== 'completed') {
      buttons.push(<button class="action-btn cancel" data-action="cancelDownload" title="Cancel">&#x2715;</button>);
    }
    return <>{buttons}</>;
  };

  return (
    <div class={`download-item${expanded() ? ' expanded' : ''}`} data-id={dl().id}>
      <div class="download-header" data-toggle onClick={() => setExpanded(!expanded())}>
        <div class="download-main">
          <span class="filename" title={dl().filename || dl().url}>
            {truncate(dl().filename || extractFilename(dl().url), 28)}
          </span>
          <div class="download-quick-stats">
            <span class="speed-compact">{formatSpeed(dl().speed)}</span>
            <span class="eta-compact">{formatETA(dl().eta)}</span>
            <span class="progress-compact">{dl().progress.toFixed(0)}%</span>
          </div>
        </div>
        <div class="download-header-right">
          <span class={`status-tag ${dl().status}`}>{statusLabel()}</span>
          <span class="expand-icon">{expanded() ? '\u25BC' : '\u25B6'}</span>
        </div>
      </div>

      <div class="download-details">
        <div class="progress-container">
          <div class="progress-bar">
            <div class="progress-fill" style={{ width: `${dl().progress}%` }} />
          </div>
          <div class="progress-text">
            <span class="size">{formatBytes(dl().downloaded)} / {formatBytes(dl().total_size)}</span>
            <span class="progress-percent">{dl().progress.toFixed(1)}%</span>
          </div>
        </div>
        <div class="download-actions" onClick={handleActionClick}>
          {renderActionButtons()}
        </div>
      </div>
    </div>
  );
}
