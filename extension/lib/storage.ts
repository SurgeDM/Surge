export const STORAGE_KEYS = {
  INTERCEPT: 'interceptEnabled',
  TOKEN: 'authToken',
  VERIFIED: 'authVerified',
  SERVER_URL: 'serverUrl',
  DISCOVERED_SERVER_URL: 'discoveredServerUrl',
  NOTIFICATIONS: 'notificationsEnabled',
  MIN_FILE_SIZE: 'minFileSize',
} as const;

export function readStoredString(
  values: Record<string, unknown>,
  key: string,
): string {
  const value = values[key];
  return typeof value === 'string' ? value : '';
}

export function readStoredBoolean(
  values: Record<string, unknown>,
  key: string,
  fallback: boolean,
): boolean {
  const value = values[key];

  if (typeof value === 'boolean') return value;
  if (value === 'true') return true;
  if (value === 'false') return false;
  return fallback;
}

export function readStoredNumber(
  values: Record<string, unknown>,
  key: string,
  fallback: number,
): number {
  const value = values[key];
  if (typeof value === 'number') return value;
  if (typeof value === 'string') {
    const parsed = parseFloat(value);
    if (!isNaN(parsed)) return parsed;
  }
  return fallback;
}
