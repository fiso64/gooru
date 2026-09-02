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
