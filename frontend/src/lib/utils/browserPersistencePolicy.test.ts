import { readdirSync, readFileSync } from 'node:fs';
import { extname, join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';

const sourceRoot = join(process.cwd(), 'src');
const allowedPersistenceModule = 'lib/utils/browserStorage.ts';
const sourceExtensions = new Set(['.ts', '.svelte']);
const forbiddenBrowserPersistence = [
  /\b(?:window\.)?localStorage\b/,
  /\b(?:window\.)?sessionStorage\b/,
  /\b(?:window\.)?indexedDB\b/,
  /\b(?:window\.)?caches\b/,
  /\bnavigator\.serviceWorker\b/
];

function productionSourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return productionSourceFiles(path);
    if (!sourceExtensions.has(extname(entry.name))) return [];
    if (entry.name.endsWith('.test.ts')) return [];
    return [path];
  });
}

describe('browser persistence architecture', () => {
  it('keeps browser persistence APIs behind the typed storage registry', () => {
    const violations = productionSourceFiles(sourceRoot).flatMap((path) => {
      const repoPath = relative(sourceRoot, path).replaceAll('\\', '/');
      if (repoPath === allowedPersistenceModule) return [];
      const source = readFileSync(path, 'utf8');
      return forbiddenBrowserPersistence
        .filter((pattern) => pattern.test(source))
        .map((pattern) => `${repoPath}: ${pattern.source}`);
    });

    expect(violations, 'direct browser persistence bypasses the protected-mode policy boundary').toEqual([]);
  });
});
