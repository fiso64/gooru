import { describe, expect, it } from 'vitest';
import { appRouteFromPath, pathForAppRoute } from './appRoute';

describe('app route URL policy', () => {
  it('gives each top-level view a stable path', () => {
    expect(pathForAppRoute('library')).toBe('/');
    expect(pathForAppRoute('upload')).toBe('/upload');
    expect(pathForAppRoute('jobs')).toBe('/jobs');
    expect(pathForAppRoute('tags')).toBe('/tags');
    expect(pathForAppRoute('settings')).toBe('/settings');
    expect(pathForAppRoute('shortcuts')).toBe('/shortcuts');
  });

  it('restores routes from direct links and trailing-slash variants', () => {
    expect(appRouteFromPath('/upload')).toBe('upload');
    expect(appRouteFromPath('/settings/')).toBe('settings');
    expect(appRouteFromPath('/')).toBe('library');
  });

  it('falls back to the library for unknown paths', () => {
    expect(appRouteFromPath('/does-not-exist')).toBe('library');
  });
});
