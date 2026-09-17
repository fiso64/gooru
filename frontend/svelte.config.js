import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: 'build',
      assets: 'build',
      fallback: 'index.html',
      precompress: false,
      strict: true
    }),
    csp: {
      mode: 'hash',
      directives: {
        'default-src': ['self'],
        'img-src': ['self', 'blob:', 'data:'],
        'media-src': ['self', 'blob:'],
        'style-src': ['self', 'unsafe-inline'],
        'script-src': ['self'],
        'connect-src': ['self'],
        'base-uri': ['self'],
        'form-action': ['self']
      }
    }
  }
};

export default config;
