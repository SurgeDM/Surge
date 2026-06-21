import { describe, expect, it } from 'vitest';
import {
  STORAGE_KEYS,
  parseServerProfiles,
  resolveActiveProfile,
  resolveActiveServerUrl,
  addServerProfile,
  removeServerProfile,
  migrateServerProfiles,
  type ServerProfile,
} from '../lib/storage';

function profile(id: string, name: string, url: string, token: string = ''): ServerProfile {
  return { id, name, url, token };
}

describe('server profile storage', () => {
  it('exposes profile storage keys', () => {
    expect(STORAGE_KEYS.PROFILES).toBe('serverProfiles');
    expect(STORAGE_KEYS.ACTIVE_PROFILE_ID).toBe('activeServerProfileId');
  });

  describe('parseServerProfiles', () => {
    it('reads a well-formed profiles array from stored values', () => {
      const values = {
        [STORAGE_KEYS.PROFILES]: [
          profile('a', 'Local', 'http://127.0.0.1:1700'),
          profile('b', 'Remote', 'http://remote:1700'),
        ],
      };
      expect(parseServerProfiles(values)).toEqual([
        profile('a', 'Local', 'http://127.0.0.1:1700'),
        profile('b', 'Remote', 'http://remote:1700'),
      ]);
    });

    it('drops malformed entries and normalizes urls', () => {
      const values = {
        [STORAGE_KEYS.PROFILES]: [
          profile('a', 'Local', '127.0.0.1:1700'),
          { id: 'b', name: 'no url' },
          { name: 'no id', url: 'http://x' },
          'garbage',
          profile('', 'empty id', 'http://y'),
        ],
      };
      expect(parseServerProfiles(values)).toEqual([
        profile('a', 'Local', 'http://127.0.0.1:1700'),
      ]);
    });

    it('returns an empty array when nothing is stored', () => {
      expect(parseServerProfiles({})).toEqual([]);
      expect(parseServerProfiles({ [STORAGE_KEYS.PROFILES]: 'nope' })).toEqual([]);
    });
  });

  describe('resolveActiveProfile', () => {
    const profiles = [
      profile('a', 'Local', 'http://127.0.0.1:1700'),
      profile('b', 'Remote', 'http://remote:1700'),
    ];

    it('returns the profile matching the active id', () => {
      expect(resolveActiveProfile(profiles, 'b')).toEqual(profiles[1]);
    });

    it('falls back to the first profile when the active id is unknown', () => {
      expect(resolveActiveProfile(profiles, 'missing')).toEqual(profiles[0]);
      expect(resolveActiveProfile(profiles, '')).toEqual(profiles[0]);
    });

    it('returns null when there are no profiles', () => {
      expect(resolveActiveProfile([], 'a')).toBeNull();
    });
  });

  describe('resolveActiveServerUrl', () => {
    it("returns the active profile's url", () => {
      const profiles = [
        profile('a', 'Local', 'http://127.0.0.1:1700'),
        profile('b', 'Remote', 'http://remote:1700'),
      ];
      expect(resolveActiveServerUrl(profiles, 'b')).toBe('http://remote:1700');
    });

    it('returns an empty string when there is no active profile', () => {
      expect(resolveActiveServerUrl([], 'b')).toBe('');
    });
  });

  describe('addServerProfile', () => {
    it('appends a normalized profile and returns it as active', () => {
      const result = addServerProfile([], { name: 'Local', url: '127.0.0.1:1700', token: 'secret' });
      expect(result.profiles).toHaveLength(1);
      expect(result.profiles[0].name).toBe('Local');
      expect(result.profiles[0].url).toBe('http://127.0.0.1:1700');
      expect(result.profiles[0].token).toBe('secret');
      expect(result.profiles[0].id).toBeTruthy();
      expect(result.activeId).toBe(result.profiles[0].id);
    });

    it('assigns unique ids across additions', () => {
      const first = addServerProfile([], { name: 'A', url: 'http://a:1700', token: '' });
      const second = addServerProfile(first.profiles, { name: 'B', url: 'http://b:1700', token: '' });
      expect(second.profiles).toHaveLength(2);
      expect(second.profiles[0].id).not.toBe(second.profiles[1].id);
      expect(second.activeId).toBe(second.profiles[1].id);
    });

    it('allows an empty string url (auto-discover)', () => {
      const profiles = addServerProfile([], { name: 'X', url: '', token: '' });
      expect(profiles.activeId).not.toBe('');
      expect(profiles.profiles[0].url).toBe('');
    });

    it('allows a whitespace only url (auto-discover)', () => {
      const profiles = addServerProfile([], { name: 'X', url: '   ', token: '' });
      expect(profiles.activeId).not.toBe('');
      expect(profiles.profiles[0].url).toBe('');
    });

    it('allows a url that normalizes to an empty string (auto-discover)', () => {
      const profiles = addServerProfile([], { name: 'X', url: '\t\n', token: '' });
      expect(profiles.activeId).not.toBe('');
      expect(profiles.profiles[0].url).toBe('');
    });
  });

  describe('removeServerProfile', () => {
    const profiles = [
      profile('a', 'Local', 'http://127.0.0.1:1700'),
      profile('b', 'Remote', 'http://remote:1700'),
      profile('c', 'Third', 'http://third:1700'),
    ];

    it('removes the profile and keeps the active id when another is active', () => {
      const result = removeServerProfile(profiles, 'a', 'b');
      expect(result.profiles.map((p) => p.id)).toEqual(['b', 'c']);
      expect(result.activeId).toBe('b');
    });

    it('re-points the active id to the first remaining profile when the active one is removed', () => {
      const result = removeServerProfile(profiles, 'b', 'b');
      expect(result.profiles.map((p) => p.id)).toEqual(['a', 'c']);
      expect(result.activeId).toBe('a');
    });

    it('returns an empty active id when the last profile is removed', () => {
      const result = removeServerProfile([profiles[0]], 'a', 'a');
      expect(result.profiles).toEqual([]);
      expect(result.activeId).toBe('');
    });
  });

  describe('migrateServerProfiles (backward compat)', () => {
    it('migrates an existing single SERVER_URL into a default profile', () => {
      const values = {
        [STORAGE_KEYS.SERVER_URL]: 'http://127.0.0.1:1700',
        [STORAGE_KEYS.TOKEN]: 'legacy-token',
      };
      const result = migrateServerProfiles(values);
      expect(result.migrated).toBe(true);
      expect(result.profiles).toHaveLength(1);
      expect(result.profiles[0].name).toBe('Default');
      expect(result.profiles[0].url).toBe('http://127.0.0.1:1700');
      expect(result.profiles[0].token).toBe('legacy-token');
      expect(result.activeId).toBe(result.profiles[0].id);
    });

    it('normalizes the legacy url during migration', () => {
      const result = migrateServerProfiles({ [STORAGE_KEYS.SERVER_URL]: '127.0.0.1:1700/' });
      expect(result.profiles[0].url).toBe('http://127.0.0.1:1700');
    });

    it('does not migrate when profiles already exist', () => {
      const values = {
        [STORAGE_KEYS.SERVER_URL]: 'http://127.0.0.1:1700',
        [STORAGE_KEYS.PROFILES]: [profile('a', 'Local', 'http://127.0.0.1:1700')],
        [STORAGE_KEYS.ACTIVE_PROFILE_ID]: 'a',
      };
      const result = migrateServerProfiles(values);
      expect(result.migrated).toBe(false);
      expect(result.profiles).toEqual([profile('a', 'Local', 'http://127.0.0.1:1700')]);
      expect(result.activeId).toBe('a');
    });

    it('does not migrate when there is no legacy url and no profiles', () => {
      const result = migrateServerProfiles({});
      expect(result.migrated).toBe(false);
      expect(result.profiles).toEqual([]);
      expect(result.activeId).toBe('');
    });

    it('resolves the active url end-to-end after migration', () => {
      const result = migrateServerProfiles({ [STORAGE_KEYS.SERVER_URL]: 'http://remote:9000' });
      expect(resolveActiveServerUrl(result.profiles, result.activeId)).toBe('http://remote:9000');
    });
  });
});
