import { describe, expect, it } from 'vitest';
import { QueryClient } from '@tanstack/query-core';
import type { FileListResponse } from '$lib/api/types';
import { fileKeys, reconcileExplicitTagMutationCache } from './files';

function list(fileID: string, tags: string[]): FileListResponse {
  return {
    files: [{
      id: fileID,
      content_id: `hash-${fileID}`,
      name: `${fileID}.jpg`,
      safe_display_path: `library/${fileID}.jpg`,
      size: 1,
      modified_time: '2026-05-20T00:00:00Z',
      media_type: 'image/jpeg',
      media_kind: 'photo',
      tags,
      media_urls: {}
    }],
    total_count: 1,
    library_count: 1
  } as FileListResponse;
}

describe('explicit tag mutation cache reconciliation', () => {
  it('patches retained unfiltered pages without changing filtered membership', () => {
    const client = new QueryClient();
    const unfiltered = fileKeys.pages(1, '', '', 'added', 'desc', 60, false, 0);
    const filtered = fileKeys.pages(1, 'blue', '', 'added', 'desc', 60, false, 0);
    client.setQueryData(unfiltered, { pages: [list('one', ['blue']), list('two', [])], pageParams: ['', '60'] });
    client.setQueryData(filtered, { pages: [list('one', ['blue'])], pageParams: [''] });

    expect(reconcileExplicitTagMutationCache(
      client,
      { operation: 'add', body: { file_ids: ['one'], tags: ['reviewed'] } },
      { operation: 'add', selector: { file_ids: ['one'] }, affected_count: 1 }
    )).toBe(true);

    const unfilteredData = client.getQueryData<{ pages: FileListResponse[] }>(unfiltered);
    const filteredData = client.getQueryData<{ pages: FileListResponse[] }>(filtered);
    expect(unfilteredData?.pages[0].files[0].tags).toEqual(['blue', 'reviewed']);
    expect(unfilteredData?.pages[1].files[0].tags).toEqual([]);
    expect(filteredData?.pages[0].files[0].tags).toEqual(['blue']);
  });

  it('requires a refresh when tagging reports a file metadata change', () => {
    const client = new QueryClient();
    const key = fileKeys.pages(1, '', '', 'added', 'desc', 60, false, 0);
    client.setQueryData(key, { pages: [list('one', ['blue'])], pageParams: [''] });

    expect(reconcileExplicitTagMutationCache(
      client,
      { operation: 'add', body: { file_ids: ['one'], tags: ['reviewed'] } },
      {
        operation: 'add',
        selector: { file_ids: ['one'] },
        affected_count: 1,
        notifications: [{ kind: 'modified', original_path: 'library/one.jpg' }]
      }
    )).toBe(false);

    const data = client.getQueryData<{ pages: FileListResponse[] }>(key);
    expect(data?.pages[0].files[0].tags).toEqual(['blue']);
  });
});
