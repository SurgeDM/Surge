import { createSignal } from 'solid-js';
import type { JSX } from 'solid-js';
import { formatSpeed, formatETA, truncate, extractFilename, formatBytes } from '../lib/utils';
import type { DownloadStatus } from '../store/types';

interface Props {
  download: DownloadStatus;
}

const STATUS_LABELS: Record<DownloadStatus['status'], string> = {
  downloading: 'Downloading',
  paused: 'Paused',
  queued: 'Queued',
  completed: 'Completed',
  error: 'Error',
};

type DownloadAction = {
  action: string;
  className: string;
  title: string;
  icon: string;
};

export default function DownloadItem(props: Props) {
  const [expanded, setExpanded] = createSignal(false);
  const dl = () => props.download;

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

  const actions = (): DownloadAction[] => {
    const status = dl().status;
    const buttons: DownloadAction[] = [];

    if (status === 'downloading') {
      buttons.push({ action: 'pauseDownload', className: 'pause', title: 'Pause', icon: '\u23F8' });
    }
    if (status === 'paused' || status === 'queued') {
      buttons.push({ action: 'resumeDownload', className: 'resume', title: 'Resume', icon: '\u25B6' });
    }
    if (status === 'completed') {
      buttons.push({ action: 'openFolder', className: 'open-folder', title: 'Open folder', icon: '\uD83D\uDCC1' });
      buttons.push({ action: 'openFile', className: 'open-file', title: 'Open file', icon: '\uD83D\uDCC4' });
    }
    if (status !== 'completed') {
      buttons.push({ action: 'cancelDownload', className: 'cancel', title: 'Cancel', icon: '\u2715' });
    }

    return buttons;
  };

  const renderActionButtons = (): JSX.Element => {
    return (
      <>
        {actions().map((button) => (
          <button class={`action-btn ${button.className}`} data-action={button.action} title={button.title}>
            {button.icon}
          </button>
        ))}
      </>
    );
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
          <span class={`status-tag ${dl().status}`}>{STATUS_LABELS[dl().status]}</span>
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
