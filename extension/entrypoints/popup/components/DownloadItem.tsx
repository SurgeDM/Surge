import { createMemo, createSignal, For } from 'solid-js';
import type { Accessor, JSX } from 'solid-js';
import { formatSpeed, formatETA, truncate, extractFilename, formatBytes, formatHistoryTimestamp } from '../lib/utils';
import type { DownloadStatus } from '../store/types';

interface Props {
  download: Accessor<DownloadStatus>;
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
  icon: JSX.Element;
};

function ActionIcon(props: { name: 'pause' | 'resume' | 'folder' | 'file' | 'cancel' }): JSX.Element {
  switch (props.name) {
    case 'pause':
      return (
        <svg viewBox="0 0 24 24" class="action-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="10" y1="5" x2="10" y2="19" />
          <line x1="14" y1="5" x2="14" y2="19" />
        </svg>
      );
    case 'resume':
      return (
        <svg viewBox="0 0 24 24" class="action-icon" fill="currentColor" aria-hidden="true">
          <path d="M8 6.5v11l8.5-5.5L8 6.5Z" />
        </svg>
      );
    case 'folder':
      return (
        <svg viewBox="0 0 24 24" class="action-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H10l2 2h6.5A2.5 2.5 0 0 1 21 9.5v7A2.5 2.5 0 0 1 18.5 19h-13A2.5 2.5 0 0 1 3 16.5v-9Z" />
        </svg>
      );
    case 'file':
      return (
        <svg viewBox="0 0 24 24" class="action-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M14 3H7.5A2.5 2.5 0 0 0 5 5.5v13A2.5 2.5 0 0 0 7.5 21h9a2.5 2.5 0 0 0 2.5-2.5V8Z" />
          <path d="M14 3v5h5" />
          <path d="M9 13h6" />
          <path d="M9 17h4" />
        </svg>
      );
    case 'cancel':
      return (
        <svg viewBox="0 0 24 24" class="action-icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="6" y1="6" x2="18" y2="18" />
          <line x1="18" y1="6" x2="6" y2="18" />
        </svg>
      );
  }
}

export default function DownloadItem(props: Props) {
  const [expanded, setExpanded] = createSignal(false);
  const dl = props.download;
  const status = createMemo(() => dl().status);
  const isCompleted = createMemo(() => status() === 'completed');
  const historyMeta = createMemo(() => {
    if (!isCompleted()) return '';

    const totalBytes = dl().total_size > 0 ? dl().total_size : dl().downloaded;
    return `${formatBytes(totalBytes)} • ${formatHistoryTimestamp(dl().added_at)}`;
  });

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

  const actions = createMemo<DownloadAction[]>(() => {
    const currentStatus = status();
    const buttons: DownloadAction[] = [];

    if (currentStatus === 'downloading') {
      buttons.push({ action: 'pauseDownload', className: 'pause', title: 'Pause', icon: <ActionIcon name="pause" /> });
    }
    if (currentStatus === 'paused' || currentStatus === 'queued') {
      buttons.push({ action: 'resumeDownload', className: 'resume', title: 'Resume', icon: <ActionIcon name="resume" /> });
    }
    if (currentStatus === 'completed') {
      buttons.push({ action: 'openFolder', className: 'open-folder', title: 'Open folder', icon: <ActionIcon name="folder" /> });
      buttons.push({ action: 'openFile', className: 'open-file', title: 'Open file', icon: <ActionIcon name="file" /> });
    }
    if (currentStatus !== 'completed') {
      buttons.push({ action: 'cancelDownload', className: 'cancel', title: 'Cancel', icon: <ActionIcon name="cancel" /> });
    }

    return buttons;
  });

  const renderActions = (inline = false) => (
    <div class={`download-actions${inline ? ' inline' : ''}`} onClick={handleActionClick}>
      <For each={actions()}>{(button) => (
        <button
          type="button"
          class={`action-btn ${button.className}`}
          data-action={button.action}
          title={button.title}
          aria-label={button.title}
        >
          {button.icon}
        </button>
      )}</For>
    </div>
  );

  if (isCompleted()) {
    return (
      <div class="download-item completed-item" data-id={dl().id}>
        <div class="download-summary">
          <div class="download-main compact">
            <span class="filename" title={dl().filename || dl().url}>
              {truncate(dl().filename || extractFilename(dl().url), 30)}
            </span>
            <span class="download-history-meta">{historyMeta()}</span>
          </div>
          {renderActions(true)}
        </div>
      </div>
    );
  }

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
          <span class={`status-tag ${status()}`}>{STATUS_LABELS[status()]}</span>
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
          <For each={actions()}>{(button) => (
            <button
              type="button"
              class={`action-btn ${button.className}`}
              data-action={button.action}
              title={button.title}
              aria-label={button.title}
            >
              {button.icon}
            </button>
          )}</For>
        </div>
      </div>
    </div>
  );
}
