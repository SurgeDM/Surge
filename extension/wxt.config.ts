import { defineConfig } from 'wxt';
import solid from 'vite-plugin-solid';

export default defineConfig({
  outDir: 'output',

  vite: () => ({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    plugins: [solid()] as any,
  }),

  manifest: ({ browser }) => {
    const base: Record<string, unknown> = {
      name: 'Surge Download Manager',
      description:
        'High-performance download acceleration with live progress tracking. Intercepts downloads and accelerates them using Surge\'s multi-connection engine.',
      permissions: ['downloads', 'storage', 'notifications', 'webRequest'],
      host_permissions: ['http://127.0.0.1/*', '<all_urls>'],
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
