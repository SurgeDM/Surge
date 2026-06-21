/**
 * Pure handler logic for SettingsView profile management.
 *
 * Each function receives its store accessors and the browser storage API as
 * explicit arguments, making the logic fully unit-testable without SolidJS
 * rendering or a DOM environment.
 */

import {
  STORAGE_KEYS,
  addServerProfile,
  removeServerProfile,
  resolveActiveServerUrl,
  resolveActiveServerToken,
  type ServerProfile,
} from '../../../lib/storage';
import { normalizeServerUrl, normalizeToken } from './utils';

export interface ProfileStore {
  getProfiles: () => ServerProfile[];
  getActiveId: () => string;
  setProfiles: (p: ServerProfile[]) => void;
  setActiveId: (id: string) => void;
  setServerUrl: (url: string) => void;
  setServerUrlLocked: (locked: boolean) => void;
  setAuthToken: (token: string) => void;
  setAuthTokenLocked: (locked: boolean) => void;
  setAuthValid: (valid: boolean) => void;
}

export interface StorageApi {
  set: (items: Record<string, unknown>) => Promise<void>;
}

/**
 * Write the new profile list and active id to storage, then update reactive
 * signals. If storage write fails the signals remain unchanged (automatic
 * rollback).
 */
export async function persistProfiles(
  profiles: ServerProfile[],
  activeId: string,
  store: ProfileStore,
  storage: StorageApi,
  authValid: boolean = false,
): Promise<void> {
  const resolvedUrl = resolveActiveServerUrl(profiles, activeId);
  const resolvedToken = resolveActiveServerToken(profiles, activeId);
  await storage.set({
    [STORAGE_KEYS.PROFILES]: profiles,
    [STORAGE_KEYS.ACTIVE_PROFILE_ID]: activeId,
    [STORAGE_KEYS.SERVER_URL]: resolvedUrl,
    [STORAGE_KEYS.TOKEN]: resolvedToken,
  });
  // Only reached when the write succeeded — update reactive state.
  store.setProfiles(profiles);
  store.setActiveId(activeId);
  store.setServerUrl(resolvedUrl);
  store.setServerUrlLocked(resolvedUrl.length > 0);
  store.setAuthToken(resolvedToken);
  store.setAuthTokenLocked(resolvedToken.length > 0);
  store.setAuthValid(authValid);
}

/**
 * Validate, create, and persist a new server profile.
 * Returns an object describing the outcome so callers can show status messages.
 */
export async function handleAddProfile(
  input: { name: string; url: string; token: string },
  store: ProfileStore,
  storage: StorageApi,
  authValid: boolean = false,
): Promise<{ ok: boolean; error?: string }> {
  const url = normalizeServerUrl(input.url);
  const token = normalizeToken(input.token);
  const name = input.name.trim() || url || 'Auto-Discover (Localhost)';
  try {
    const { profiles, activeId } = addServerProfile(store.getProfiles(), { name, url, token });
    await persistProfiles(profiles, activeId, store, storage, authValid);
    return { ok: true };
  } catch {
    return { ok: false, error: 'Failed to save' };
  }
}

/**
 * Switch the active profile and persist the change.
 */
export async function handleSwitchProfile(
  profileId: string,
  store: ProfileStore,
  storage: StorageApi,
): Promise<{ ok: boolean; error?: string }> {
  try {
    await persistProfiles(store.getProfiles(), profileId, store, storage);
    return { ok: true };
  } catch {
    return { ok: false, error: 'Failed to switch' };
  }
}

/**
 * Remove the currently-active profile and persist the change.
 */
export async function handleDeleteProfile(
  store: ProfileStore,
  storage: StorageApi,
): Promise<{ ok: boolean; error?: string }> {
  try {
    const { profiles, activeId } = removeServerProfile(
      store.getProfiles(),
      store.getActiveId(),
      store.getActiveId(),
    );
    await persistProfiles(profiles, activeId, store, storage);
    return { ok: true };
  } catch {
    return { ok: false, error: 'Failed to remove' };
  }
}
