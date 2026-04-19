/**
 * Type definitions matching the Surge Go backend structs.
 * All byte values are in bytes; speed in the API is MB/s per DownloadStatus.
 */

export interface DownloadStatus {
  id: string;
  url: string;
  filename: string;
  dest_path?: string;
  total_size: number;
  downloaded: number;
  progress: number;           // percentage 0-100
  speed: number;              // MB/s
  status: 'queued' | 'paused' | 'downloading' | 'completed' | 'error';
  error?: string;
  eta: number;               // seconds remaining
  connections: number;       // active connections
  added_at: number;          // unix timestamp
  time_taken: number;        // milliseconds (completed only)
  avg_speed: number;         // bytes/sec (completed only)
}

export interface HistoryEntry {
  id: string;
  url: string;
  filename: string;
  dest_path: string;
  status: string;
  total_size: number;
  downloaded: number;
  completed_at: number;      // unix seconds
  time_taken: number;
  avg_speed: number;         // bytes/sec
}

// === SSE Event Types (from internal/engine/events/events.go) ===

export interface ProgressMsg {
  DownloadID: string;
  Downloaded: number;
  Total: number;
  Speed: number;
  Elapsed: number;
  ActiveConnections: number;
}

export interface DownloadStartedMsg {
  DownloadID: string;
  URL: string;
  Filename: string;
  Total: number;
  DestPath: string;
}

export interface DownloadCompleteMsg {
  DownloadID: string;
  Filename: string;
  Elapsed: number;
  Total: number;
  AvgSpeed: number;
}

export interface DownloadErrorMsg {
  DownloadID: string;
  Filename: string;
  DestPath: string;
  Err: string;
}

export interface DownloadPausedMsg {
  DownloadID: string;
  Filename: string;
  Downloaded: number;
}

export interface DownloadResumedMsg {
  DownloadID: string;
  Filename: string;
}

export interface DownloadQueuedMsg {
  DownloadID: string;
  Filename: string;
  URL: string;
  DestPath: string;
  Mirrors: string[];
}

export interface DownloadRemovedMsg {
  DownloadID: string;
  Filename: string;
  DestPath: string;
  Completed: boolean;
}

