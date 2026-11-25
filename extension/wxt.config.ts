import { execFileSync } from 'node:child_process';
import { defineConfig } from 'wxt';
import solid from 'vite-plugin-solid';

function getLocalCommit(): string {
  if (process.env.CI) return '';

  try {
    return execFileSync('git', ['rev-parse', '--short=12', 'HEAD'], {
      cwd: new URL('.', import.meta.url),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    return '';
  }
}

const localCommit = getLocalCommit();

export default defineConfig({
  outDir: 'output',

  vite: () => ({
    define: { __SURGE_LOCAL_COMMIT__: JSON.stringify(localCommit) },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    plugins: [solid()] as any,
  }),

  hooks: {
    'build:manifestGenerated': (wxt, manifest) => {
      if (localCommit && wxt.config.browser !== 'firefox') {
        manifest.version_name = localCommit;
      }
    },
  },

  manifest: ({ browser }) => {
    const base: Record<string, unknown> = {
      name: 'Surge Download Manager',
      description:
        'High-performance download acceleration. Intercepts and routes browser downloads to Surge\'s multi-connection engine.',
      permissions: ['downloads', 'storage', 'notifications', 'webRequest'],
      host_permissions: ['http://127.0.0.1/*', '<all_urls>'],
      icons: {
        16: 'icons/icon16.png',
        48: 'icons/icon48.png',
        128: 'icons/icon128.png',
      },
      action: {
        default_icon: {
          16: 'icons/icon16.png',
          48: 'icons/icon48.png',
          128: 'icons/icon128.png',
        },
      },
      browser_action: {
        default_icon: {
          16: 'icons/icon16.png',
          48: 'icons/icon48.png',
          128: 'icons/icon128.png',
        },
      },
    };

    if (browser === 'firefox') {
      return {
        ...base,
        content_security_policy: {
          extension_pages: "script-src 'self'; object-src 'self'",
        },
        browser_specific_settings: {
          gecko: {
            id: 'surge@surge-downloader.com',
            strict_min_version: '109.0',
          },
        },
      };
    }

    return base;
  },
});
