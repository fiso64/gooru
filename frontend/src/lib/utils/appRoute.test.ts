import { describe, expect, it } from 'vitest';
import {
  appRouteFromPath,
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
    expect(appRouteFromPath('/settings/')).toBe('settings');
    expect(appRouteFromPath('/')).toBe('library');
  });

  it('falls back to the library for unknown paths', () => {
    expect(appRouteFromPath('/does-not-exist')).toBe('library');
  });

  it('round-trips meaningful library state while omitting defaults', () => {
    const search = searchForLibraryURLState({ query: 'artist:foo bar', kind: 'photo', sort: 'name', order: 'asc' });
    expect(search).toBe('?q=artist%3Afoo+bar&type=photo&sort=name&order=asc');
    expect(libraryURLStateFromSearch(search)).toEqual({
      query: 'artist:foo bar',
      kind: 'photo',
      sort: 'name',
      order: 'asc'
    });
    expect(searchForLibraryURLState({ query: '', kind: '', sort: 'modified', order: 'desc' })).toBe('');
  });

  it('normalizes invalid URL state to safe library defaults', () => {
    expect(libraryURLStateFromSearch('?sort=wat&order=sideways&q=%20fox%20')).toEqual({
      query: 'fox',
      kind: '',
      sort: 'modified',
      order: 'desc'
    });
  });
});
