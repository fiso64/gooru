export function hasDraggedFiles(types: ArrayLike<string> | Iterable<string> | null | undefined): boolean {
  return Array.from(types ?? []).includes('Files');
}

export function isPasteableMediaFile(file: Pick<File, 'type'>): boolean {
  const type = file.type.trim().toLowerCase();
  return type.startsWith('image/') || type.startsWith('video/');
}

export function pastedMediaFiles(files: ArrayLike<File> | Iterable<File> | null | undefined): File[] {
  return Array.from(files ?? []).filter(isPasteableMediaFile);
}

export function clipboardMediaFiles(
  transfer: Pick<DataTransfer, 'files' | 'items'> | null | undefined
): File[] {
  if (!transfer) return [];

  const direct = pastedMediaFiles(transfer.files);
  if (direct.length) return direct;

  const itemFiles = Array.from(transfer.items ?? [])
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file));
  return pastedMediaFiles(itemFiles);
}
