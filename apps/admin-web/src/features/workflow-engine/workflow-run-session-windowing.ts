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
