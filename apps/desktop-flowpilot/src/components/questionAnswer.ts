export function resolveQuestionManualSubmit(
  selected: string[],
  other: string,
  multiSelect?: boolean,
): string | string[] | undefined {
  const typed = other.trim();

  if (!multiSelect) {
    return typed || undefined;
  }

  const picks = [...selected];
  if (typed) picks.push(typed);
  return picks.length > 0 ? picks : undefined;
}
