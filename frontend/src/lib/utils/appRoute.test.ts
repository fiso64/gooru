import { describe, expect, it } from 'vitest';
import {
  appRouteFromPath,
  kitRouteForAppPath,
  libraryURLStateFromSearch,
  pathForAppRoute,
  searchForLibraryURLState
} from './appRoute';

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
    expect(appRouteFromPath('/uploads')).toBe('upload');
    expect(appRouteFromPath('/settings/')).toBe('settings');
    expect(appRouteFromPath('/')).toBe('library');
  });

  it('reroutes known client-side views through the single SvelteKit shell route', () => {
    expect(kitRouteForAppPath('/tags')).toBe('/');
    expect(kitRouteForAppPath('/jobs/')).toBe('/');
    expect(kitRouteForAppPath('/upload')).toBe('/');
    expect(kitRouteForAppPath('/uploads')).toBe('/');
    expect(kitRouteForAppPath('/settings')).toBe('/');
    expect(kitRouteForAppPath('/does-not-exist')).toBe('/does-not-exist');
    expect(kitRouteForAppPath('/_app/immutable/app.js')).toBe('/_app/immutable/app.js');
  });

  it('falls back to the library for unknown paths', () => {
    expect(appRouteFromPath('/does-not-exist')).toBe('library');
  });

  it('round-trips meaningful library and preview state while omitting defaults', () => {
    const search = searchForLibraryURLState({ query: 'artist:foo bar', kind: 'photo', sort: 'name', order: 'asc', fileID: 'opaque-file' });
    expect(search).toBe('?q=artist%3Afoo+bar&type=photo&sort=name&order=asc&file=opaque-file');
    expect(libraryURLStateFromSearch(search)).toEqual({
      query: 'artist:foo bar',
      kind: 'photo',
      sort: 'name',
      order: 'asc',
      fileID: 'opaque-file'
    });
    expect(searchForLibraryURLState({ query: '', kind: '', sort: 'modified', order: 'desc', fileID: '' })).toBe('');
  });

  it('normalizes invalid URL state to safe library defaults', () => {
    expect(libraryURLStateFromSearch('?sort=wat&order=sideways&q=%20fox%20')).toEqual({
      query: 'fox',
      kind: '',
      sort: 'modified',
      order: 'desc',
      fileID: ''
    });
  });
});
