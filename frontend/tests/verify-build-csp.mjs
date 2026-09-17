import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';

const html = await readFile(new URL('../build/index.html', import.meta.url), 'utf8');

function attribute(tag, name) {
  const match = tag.match(new RegExp(`\\b${name}\\s*=\\s*(["'])(.*?)\\1`, 'i'));
  return match?.[2] ?? null;
}

const cspMeta = [...html.matchAll(/<meta\b[^>]*>/gi)].find(
  ([tag]) => attribute(tag, 'http-equiv')?.toLowerCase() === 'content-security-policy'
)?.[0];

if (!cspMeta) {
  throw new Error('built index.html is missing a Content-Security-Policy meta tag');
}

const csp = attribute(cspMeta, 'content');
if (!csp) {
  throw new Error('built index.html CSP meta tag is missing content');
}
for (const directive of ["default-src 'self'", "script-src 'self'", "connect-src 'self'"]) {
  if (!csp.includes(directive)) {
    throw new Error(`built index.html CSP is missing ${directive}: ${csp}`);
  }
}
if (csp.includes('nonce-')) {
  throw new Error(`prerendered CSP should use build-time hashes, not nonces: ${csp}`);
}

const inlineScripts = [...html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)]
  .filter(([, attrs]) => !/\bsrc\s*=/i.test(attrs))
  .map(([, , body]) => body);

for (const body of inlineScripts) {
  const hash = createHash('sha256').update(body).digest('base64');
  const source = `'sha256-${hash}'`;
  if (!csp.includes(source)) {
    throw new Error(`built index.html CSP is missing hash for generated inline script: ${source}`);
  }
}

console.log(`verified build-owned CSP for index.html (${inlineScripts.length} inline script(s))`);
