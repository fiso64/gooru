export function isViewerTagTextKey(key: string): boolean {
  return (key.length === 1 && key !== ' ') || key === 'Dead' || key === 'Process' || key === 'Unidentified';
}
