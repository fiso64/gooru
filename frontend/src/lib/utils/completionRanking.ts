export type CompletionCandidate = {
  name: string;
  count?: number;
};

export function completionComponentStartsWith(completion: string, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return false;
  return completion
    .toLowerCase()
    .split(/[:_]/)
    .some((component) => component.startsWith(needle));
}

export function compareCompletionRank(
  aName: string,
  bName: string,
  query: string,
  aCount = 0,
  bCount = 0
): number {
  const aPrefix = completionComponentStartsWith(aName, query);
  const bPrefix = completionComponentStartsWith(bName, query);
  if (aPrefix !== bPrefix) return aPrefix ? -1 : 1;
  return bCount - aCount || aName.localeCompare(bName);
}

export function rankCompletionCandidates<T extends CompletionCandidate>(candidates: T[], query: string): T[] {
  return [...candidates].sort((a, b) => compareCompletionRank(a.name, b.name, query, a.count ?? 0, b.count ?? 0));
}
