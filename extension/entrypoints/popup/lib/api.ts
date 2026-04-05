/**
 * API client for communicating with the Surge HTTP backend.
 * Used by both background (direct fetch) and popup (via background messaging).
 *
 * All methods accept an optional serverUrl and authToken; when omitted,
 * the values from storage are used.
 */

import type { DownloadStatus, HistoryEntry } from '../store/types';

const API_BASE = 'http://127.0.0.1:1700';

export interface SurgeApiClient {
  serverUrl: () => string;
  authToken: () => string;
}

async function apiFetch<T>(
  client: SurgeApiClient,
  path: string,
  options?: RequestInit,
): Promise<{ ok: boolean; data?: T; status?: number }> {
  const baseUrl = client.serverUrl() || API_BASE;
  const token = client.authToken();
  const url = `${baseUrl}${path}`;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(options?.headers as Record<string, string> || {}),
  };

  try {
    const response = await fetch(url, { ...options, headers, signal: AbortSignal.timeout(5000) });
    if (response.ok) {
      const contentType = response.headers.get('content-type') || '';
      const data = contentType.includes('application/json')
        ? await response.json()
        : undefined;
      return { ok: true, data, status: response.status };
    }
    return { ok: false, status: response.status };
  } catch {
    return { ok: false };
  }
}

export async function checkHealth(client: SurgeApiClient): Promise<boolean> {
  const baseUrl = client.serverUrl() || API_BASE;
  try {
    const response = await fetch(`${baseUrl}/health`, {
      method: 'GET',
      signal: AbortSignal.timeout(1000),
    });
    if (response.ok) {
      const contentType = response.headers.get('content-type') || '';
      if (contentType.includes('application/json')) {
        const data = await response.json().catch(() => null);
        return data?.status === 'ok';
      }
    }
  } catch {
    // ignore
  }
  return false;
}

export async function fetchDownloads(client: SurgeApiClient): Promise<DownloadStatus[]> {
  const result = await apiFetch<DownloadStatus[]>(client, '/list');
  return Array.isArray(result.data) ? result.data : [];
}

export async function fetchHistory(client: SurgeApiClient): Promise<HistoryEntry[]> {
  const result = await apiFetch<HistoryEntry[]>(client, '/history');
  return Array.isArray(result.data) ? result.data : [];
}

export async function sendDownload(
  client: SurgeApiClient,
  url: string,
  filename?: string,
  path?: string,
  headers?: Record<string, string>,
): Promise<{ success: boolean; error?: string }> {
  const baseUrl = client.serverUrl() || API_BASE;
  const token = client.authToken();

  const body: Record<string, unknown> = { url, filename: filename || '' };
  if (path) body.path = path;
  if (headers) body.headers = headers;

  try {
    const response = await fetch(`${baseUrl}/download`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) {
      return { success: true };
    }
    if (response.status === 409) {
      const text = await response.text().catch(() => '');
      try {
        const json = JSON.parse(text);
        return { success: false, error: json.message || text };
      } catch {
        return { success: false, error: text };
      }
    }
    const text = await response.text().catch(() => '');
    return { success: false, error: text };
  } catch (error) {
    return { success: false, error: error instanceof Error ? error.message : String(error) };
  }
}

async function apiAction(
  client: SurgeApiClient,
  method: string,
  path: string,
): Promise<boolean> {
  const result = await apiFetch(client, path, { method });
  return result.ok;
}

export async function pauseDownload(client: SurgeApiClient, id: string): Promise<boolean> {
  return apiAction(client, 'POST', `/pause?id=${encodeURIComponent(id)}`);
}

export async function resumeDownload(client: SurgeApiClient, id: string): Promise<boolean> {
  return apiAction(client, 'POST', `/resume?id=${encodeURIComponent(id)}`);
}

export async function cancelDownload(client: SurgeApiClient, id: string): Promise<boolean> {
  return apiAction(client, 'DELETE', `/delete?id=${encodeURIComponent(id)}`);
}

export async function openFile(client: SurgeApiClient, id: string): Promise<boolean> {
  return apiAction(client, 'POST', `/open-file?id=${encodeURIComponent(id)}`);
}

export async function openFolder(client: SurgeApiClient, id: string): Promise<boolean> {
  return apiAction(client, 'POST', `/open-folder?id=${encodeURIComponent(id)}`);
}

export async function validateAuth(client: SurgeApiClient): Promise<{ ok: boolean; status?: number; error?: string }> {
  const result = await apiFetch(client, '/list');
  if (result.ok) return { ok: true };
  if (result.status === 401 || result.status === 403) {
    return { ok: false, status: result.status };
  }
  return { ok: false, error: 'no_server' };
}
