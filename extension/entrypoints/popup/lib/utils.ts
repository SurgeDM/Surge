/**
 * Utility functions for formatting and parsing download data.
 */

export const KB = 1 << 10;
export const MB = 1 << 20;

export function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(KB)), units.length - 1);
  const value = bytes / Math.pow(KB, i);
  return value.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

export function formatSpeed(mbps: number): string {
  if (!mbps || mbps <= 0) return '--';
  if (mbps < 0.01) return (mbps * MB).toFixed(0) + ' B/s';
  if (mbps < 1) return (mbps * KB).toFixed(1) + ' KB/s';
  return mbps.toFixed(1) + ' MB/s';
}

export function formatETA(seconds: number): string {
  if (!seconds || seconds <= 0) return '--:--';
  if (seconds > 604800) return '> 1 week';
  if (seconds > 86400) return '> 1 day';

  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export function truncate(str: string, len: number): string {
  if (!str) return 'Unknown';
  return str.length > len ? str.slice(0, len - 3) + '...' : str;
}

export function extractFilename(url: string): string {
  if (!url) return 'Unknown';
  try {
    const pathname = new URL(url).pathname;
    const filename = pathname.split('/').pop();
    return decodeURIComponent(filename || '') || 'Unknown';
  } catch {
    return url.split('/').pop() || 'Unknown';
  }
}

export function normalizeToken(token: string | undefined): string {
  if (!token) return '';
  return token.replace(/\s+/g, '');
}

export function normalizeServerUrl(url: string): string {
  if (!url) return '';
  url = url.trim();
  if (url && !/^https?:\/\//i.test(url)) url = 'http://' + url;
  return url.replace(/\/+$/, '');
}
