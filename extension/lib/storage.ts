export const STORAGE_KEYS = {
  INTERCEPT: 'interceptEnabled',
  TOKEN: 'authToken',
  VERIFIED: 'authVerified',
  SERVER_URL: 'serverUrl',
  DISCOVERED_SERVER_URL: 'discoveredServerUrl',
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
