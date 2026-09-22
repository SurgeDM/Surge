import { beforeEach, describe, expect, it } from 'vitest';
import { MB, formatSpeed } from '../entrypoints/popup/lib/utils';
import {
  activeDownloads,
  handleSseEvent,
  reconcileActiveDownloads,
  setActiveDownloads,
} from '../entrypoints/popup/store';
import type { DownloadStatus } from '../entrypoints/popup/store/types';

const download: DownloadStatus = {
  id: 'download-1',
  url: 'https://example.com/file',
  filename: 'file',
  total_size: 100 * MB,
  downloaded: 10 * MB,
  progress: 10,
  speed: 8 * MB, // /list reports bytes/s
  status: 'downloading',
  eta: 0,
  connections: 1,
  added_at: 0,
  time_taken: 0,
  avg_speed: 0,
};

describe('active download speed', () => {
  beforeEach(() => setActiveDownloads([]));

  it('normalizes /list speed on initial load and after an SSE update', () => {
    reconcileActiveDownloads([download]);
    expect(formatSpeed(activeDownloads()[0].speed)).toBe('8.0 MB/s');

    handleSseEvent('progress', {
      DownloadID: download.id,
      Downloaded: 20 * MB,
      Total: download.total_size,
      Speed: 4 * MB,
      ActiveConnections: 1,
    });
    expect(formatSpeed(activeDownloads()[0].speed)).toBe('4.0 MB/s');

    reconcileActiveDownloads([download]);
    expect(formatSpeed(activeDownloads()[0].speed)).toBe('8.0 MB/s');
  });
});
