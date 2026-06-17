export const SESSION_PROMPT_GROUP_PAGE_SIZE = 6;

export function getVisiblePromptGroupSlice<T>(
  promptGroups: T[],
  visibleCount: number,
) {
  if (visibleCount >= promptGroups.length) {
    return promptGroups;
  }

  return promptGroups.slice(Math.max(promptGroups.length - visibleCount, 0));
}

export function getNextPromptGroupVisibleCount({
  currentCount,
  pageSize = SESSION_PROMPT_GROUP_PAGE_SIZE,
  totalCount,
}: {
  currentCount: number;
  pageSize?: number;
  totalCount: number;
}) {
  return Math.min(totalCount, currentCount + pageSize);
}

export function getTotalStepPromptGroupCount<G extends { promptGroups: unknown[] }>(
  sessionGroups: G[],
): number {
  return sessionGroups.reduce((sum, g) => sum + g.promptGroups.length, 0);
}

export function getVisibleStepSessionGroupSlice<G extends { promptGroups: PG[] }, PG>(
  sessionGroups: G[],
  visiblePromptGroupCount: number,
): G[] {
  const total = getTotalStepPromptGroupCount(sessionGroups);
  if (visiblePromptGroupCount >= total) {
    return sessionGroups;
  }

  let remaining = visiblePromptGroupCount;
  const result: G[] = [];

  for (let i = sessionGroups.length - 1; i >= 0 && remaining > 0; i--) {
    const group = sessionGroups[i];
    const take = Math.min(remaining, group.promptGroups.length);
    result.unshift({
      ...group,
      promptGroups: group.promptGroups.slice(group.promptGroups.length - take),
    } as G);
    remaining -= take;
  }

  return result;
}
