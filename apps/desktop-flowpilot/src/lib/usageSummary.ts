// Context-window remaining % and account credit chips for the chat usage line
// (Grok CLI `/usage` parity: weekly remaining + context remaining + input tokens).
// No USD/cost: Grok ACP `_meta` has no dollar field, and inventing a price would lie.

export function contextRemainingPercent(used: number, windowSize: number): number {
  if (!(windowSize > 0)) return 0;
  const remaining = Math.max(windowSize - used, 0);
  return Math.round((remaining / windowSize) * 100);
}

export type AccountRemainingSource = {
  remaining7dPercent: number | null;
  remaining5hPercent: number | null;
  usageDetailLines?: { label: string; remainingPercent: number }[];
};

export function formatAccountRemainingLabel(account: AccountRemainingSource | undefined | null): string | null {
  if (!account) return null;
  if (account.remaining7dPercent != null) {
    return `7d ${account.remaining7dPercent}%`;
  }
  if (account.remaining5hPercent != null) {
    return `5h ${account.remaining5hPercent}%`;
  }
  const line = account.usageDetailLines?.[0];
  if (line && Number.isFinite(line.remainingPercent)) {
    const label = line.label.trim() || "quota";
    return `${label} ${line.remainingPercent}%`;
  }
  return null;
}
