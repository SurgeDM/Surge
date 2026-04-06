export interface PendingDup {
  url: string;
  filename: string;
  directory: string;
  timestamp: number;
}

export function coerceStoredBoolean(value: unknown): boolean | undefined {
  if (typeof value === 'boolean') return value;
  if (value === 'true') return true;
  if (value === 'false') return false;
  return undefined;
}

export function resolveInterceptEnabled(value: unknown): boolean {
  return coerceStoredBoolean(value) ?? true;
}

export function buildEventStreamHeaders(authToken: string | null): Record<string, string> {
  return {
    Accept: 'text/event-stream',
    'Cache-Control': 'no-cache',
    ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
  };
}

export async function openEventStream(
  baseUrl: string,
  authToken: string | null,
  signal: AbortSignal,
  fetchImpl: typeof fetch = fetch,
): Promise<Response> {
  return fetchImpl(`${baseUrl}/events`, {
    headers: buildEventStreamHeaders(authToken),
    signal,
  });
}

interface QueueDuplicateDownloadOptions {
  pendingDuplicates: Map<string, PendingDup>;
  pendingDuplicateCounter: number;
  url: string;
  filename: string;
  directory: string;
  persistPendingDuplicates: () => Promise<void>;
  updateBadge: () => void;
  openPopup: () => Promise<void>;
  sendPrompt: (message: { type: 'promptDuplicate'; id: string; filename: string }) => Promise<unknown>;
  cleanupStaleDuplicates?: () => void;
  now?: () => number;
}

export async function queueDuplicateDownload(opts: QueueDuplicateDownloadOptions): Promise<number> {
  const nextCounter = opts.pendingDuplicateCounter + 1;
  const pendingId = `dup_${nextCounter}`;
  opts.pendingDuplicates.set(pendingId, {
    url: opts.url,
    filename: opts.filename,
    directory: opts.directory,
    timestamp: opts.now?.() ?? Date.now(),
  });

  opts.cleanupStaleDuplicates?.();
  await opts.persistPendingDuplicates();
  opts.updateBadge();

  try {
    await opts.openPopup();
  } catch {
    // Ignore popup failures in background contexts where the browser forbids opening it.
  }

  opts.sendPrompt({ type: 'promptDuplicate', id: pendingId, filename: opts.filename }).catch(() => {});
  return nextCounter;
}
