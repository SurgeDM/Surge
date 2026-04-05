/**
 * Reactive store using SolidJS signals.
 * Uses plain objects/arrays instead of Map for proper Solid reactivity.
 */

import { createSignal } from 'solid-js';
import type {
  DownloadStatus,
  HistoryEntry,
  ProgressMsg,
  DownloadStartedMsg,
  DownloadCompleteMsg,
  DownloadErrorMsg,
  DownloadPausedMsg,
  DownloadResumedMsg,
  DownloadQueuedMsg,
  DownloadRemovedMsg,
} from './types';

const MB = 1 << 20;

// --- Active downloads (array of DownloadStatus) ---
const [activeDownloads, setActiveDownloads] = createSignal<DownloadStatus[]>([]);
export { activeDownloads, setActiveDownloads };

export function upsertActiveDownload(dl: DownloadStatus): void {
  setActiveDownloads(prev => {
    const idx = prev.findIndex(d => d.id === dl.id);
    if (idx >= 0) {
      const next = [...prev];
      next[idx] = { ...next[idx], ...dl };
      return next;
    }
    return [...prev, dl];
  });
}

export function removeActiveDownload(id: string): void {
  setActiveDownloads(prev => prev.filter(d => d.id !== id));
}

// --- History ---
const [historyDownloads, setHistoryDownloads] = createSignal<HistoryEntry[]>([]);
export { historyDownloads, setHistoryDownloads };

// --- Connection status ---
const [serverConnected, setServerConnected] = createSignal(false);
export { serverConnected, setServerConnected };

// --- Intercept toggle ---
const [interceptEnabled, setInterceptEnabled] = createSignal(true);
export { interceptEnabled, setInterceptEnabled };

// --- Settings ---
const [serverUrl, setServerUrl] = createSignal('');
export { serverUrl, setServerUrl };

const [authToken, setAuthToken] = createSignal('');
export { authToken, setAuthToken };

// --- Auth state ---
const [authValid, setAuthValid] = createSignal(false);
export { authValid, setAuthValid };

const [authValidationInProgress, setAuthValidationInProgress] = createSignal(false);
export { authValidationInProgress, setAuthValidationInProgress };

// --- View ---
export type ViewMode = 'active' | 'history';
const [currentView, setCurrentView] = createSignal<ViewMode>('active');
export { currentView, setCurrentView };

// --- SSE event handling ---

export function handleSseEvent(event: string, data: unknown): void {
  switch (event) {
    case 'progress': {
      const msg = data as ProgressMsg;
      const list = activeDownloads();
      const idx = list.findIndex(d => d.id === msg.DownloadID);
      if (idx >= 0) {
        const existing = list[idx];
        const totalBytes = msg.Total || existing.total_size;
        const downloaded = msg.Downloaded;
        const progress = totalBytes > 0 ? (downloaded / totalBytes) * 100 : 0;
        const speedMBps = msg.Speed / MB;
        const eta = msg.Speed > 0 ? Math.ceil((totalBytes - downloaded) / msg.Speed) : 0;

        setActiveDownloads(prev => {
          const next = [...prev];
          next[idx] = {
            ...existing,
            downloaded,
            progress,
            speed: speedMBps,
            eta,
            connections: msg.ActiveConnections,
            total_size: totalBytes,
          };
          return next;
        });
      }
      break;
    }
    case 'started': {
      const msg = data as DownloadStartedMsg;
      const list = activeDownloads();
      const existing = list.find(d => d.id === msg.DownloadID);
      if (existing) {
        // Transition from queued to downloading
        upsertActiveDownload({
          ...existing,
          status: 'downloading',
          downloaded: 0,
          progress: 0,
          total_size: msg.Total,
          dest_path: msg.DestPath,
        });
      } else {
        upsertActiveDownload({
          id: msg.DownloadID,
          url: msg.URL,
          filename: msg.Filename,
          dest_path: msg.DestPath,
          total_size: msg.Total,
          downloaded: 0,
          progress: 0,
          speed: 0,
          status: 'downloading',
          eta: 0,
          connections: 0,
          added_at: Date.now(),
          time_taken: 0,
          avg_speed: 0,
        });
      }
      break;
    }
    case 'complete': {
      const msg = data as DownloadCompleteMsg;
      const list = activeDownloads();
      const idx = list.findIndex(d => d.id === msg.DownloadID);
      if (idx >= 0) {
        const existing = list[idx];
        setActiveDownloads(prev => {
          const next = [...prev];
          next[idx] = {
            ...existing,
            status: 'completed',
            progress: 100,
            downloaded: msg.Total,
            avg_speed: msg.AvgSpeed,
            time_taken: msg.Elapsed,
          };
          return next;
        });
      }
      break;
    }
    case 'error': {
      const msg = data as DownloadErrorMsg;
      const list = activeDownloads();
      const idx = list.findIndex(d => d.id === msg.DownloadID);
      if (idx >= 0) {
        const existing = list[idx];
        setActiveDownloads(prev => {
          const next = [...prev];
          next[idx] = { ...existing, status: 'error', error: msg.Err };
          return next;
        });
      }
      break;
    }
    case 'paused': {
      const msg = data as DownloadPausedMsg;
      const list = activeDownloads();
      const idx = list.findIndex(d => d.id === msg.DownloadID);
      if (idx >= 0) {
        const existing = list[idx];
        setActiveDownloads(prev => {
          const next = [...prev];
          next[idx] = {
            ...existing,
            status: 'paused',
            downloaded: msg.Downloaded,
            progress: existing.total_size > 0 ? (msg.Downloaded / existing.total_size) * 100 : 0,
          };
          return next;
        });
      }
      break;
    }
    case 'resumed': {
      const msg = data as DownloadResumedMsg;
      const list = activeDownloads();
      const idx = list.findIndex(d => d.id === msg.DownloadID);
      if (idx >= 0) {
        const existing = list[idx];
        setActiveDownloads(prev => {
          const next = [...prev];
          next[idx] = { ...existing, status: 'downloading' };
          return next;
        });
      }
      break;
    }
    case 'queued': {
      const msg = data as DownloadQueuedMsg;
      const list = activeDownloads();
      const existing = list.find(d => d.id === msg.DownloadID);
      if (!existing) {
        upsertActiveDownload({
          id: msg.DownloadID,
          url: msg.URL,
          filename: msg.Filename,
          dest_path: msg.DestPath,
          total_size: 0,
          downloaded: 0,
          progress: 0,
          speed: 0,
          status: 'queued',
          eta: 0,
          connections: 0,
          added_at: Date.now(),
          time_taken: 0,
          avg_speed: 0,
        });
      }
      break;
    }
    case 'removed': {
      const msg = data as DownloadRemovedMsg;
      removeActiveDownload(msg.DownloadID);
      break;
    }
  }
}
