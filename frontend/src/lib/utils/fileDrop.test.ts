import { describe, expect, it } from 'vitest';
import { clipboardMediaFiles, hasDraggedFiles, isPasteableMediaFile, pastedMediaFiles } from './fileDrop';

function file(type: string, name: string) {
  return { type, name } as File;
}

function item(fileValue: File | null): DataTransferItem {
  return {
    kind: 'file',
    type: fileValue?.type ?? '',
    getAsFile: () => fileValue
  } as DataTransferItem;
}

describe('global file ingress', () => {
  it('recognizes file drag payloads', () => {
    expect(hasDraggedFiles(['text/plain', 'Files'])).toBe(true);
    expect(hasDraggedFiles(['text/plain'])).toBe(false);
    expect(hasDraggedFiles(null)).toBe(false);
  });

  it('accepts pasted image, gif, and video files', () => {
    expect(isPasteableMediaFile(file('image/png', 'image.png'))).toBe(true);
    expect(isPasteableMediaFile(file('image/gif', 'animated.gif'))).toBe(true);
    expect(isPasteableMediaFile(file('video/mp4', 'clip.mp4'))).toBe(true);
  });

  it('leaves non-media clipboard files out of the upload ingress', () => {
    const image = file('image/jpeg', 'photo.jpg');
    const text = file('text/plain', 'notes.txt');
    const archive = file('application/zip', 'files.zip');

    expect(pastedMediaFiles([text, image, archive])).toEqual([image]);
  });

  it('falls back to clipboard items when files are not populated', () => {
    const gif = file('image/gif', 'animated.gif');
    const text = file('text/plain', 'notes.txt');
    const transfer = {
      files: [] as unknown as FileList,
      items: [item(text), item(gif)] as unknown as DataTransferItemList
    };

    expect(clipboardMediaFiles(transfer)).toEqual([gif]);
  });

  it('prefers clipboard files over item fallback to avoid duplicates', () => {
    const image = file('image/png', 'image.png');
    const transfer = {
      files: [image] as unknown as FileList,
      items: [item(image)] as unknown as DataTransferItemList
    };

    expect(clipboardMediaFiles(transfer)).toEqual([image]);
  });
});
