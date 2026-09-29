import type { RemoteChatSessionSummary, RunHistoryItem } from "@/types/contract";

export function isProjectSyncing(history: RunHistoryItem[], projectId: string): boolean {
  return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}

export function isAgentHistoryItem(item: RunHistoryItem): boolean {
  return Boolean(item.parentRunId || hasBuiltInAgentPromptPrefix(item.lastPrompt));
}

// isSyncableRun reports whether a top-level run still needs a Sync-to-Drive
// action. "chat" is a normal chat run; "" / "workflow" is a flow-engine run's
// own hub (Task-190 / CP-36 P-5) -- its children are already runKind "chat"
// (spawned via ChatMode=normal_chat), so they are covered by the first case.
// Mirrors isSyncableRunKind in chat_session_sync.go; keep both in sync.
//
// remoteChatSessions is an optional reconciliation source (the confirmed Drive
// index, when already loaded): a local syncStatus flag can go stale relative
// to it -- e.g. a background history poll can win a race against a just-set
// "synced" flag and briefly overwrite it with the pre-sync snapshot it fetched
// -- which would otherwise leave a "needs sync" badge stuck on a chat that is
// already safely in Drive. When provided, a run whose sourceMachineId/
// sourceRunId already appears remotely is treated as synced regardless of what
// the local flag currently says.
export function isSyncableRun(item: RunHistoryItem, remoteChatSessions?: RemoteChatSessionSummary[]): boolean {
  const runKind = item.runKind ?? "";
  const isSyncableKind = runKind === "chat" || runKind === "" || runKind === "workflow";
  if (!isSyncableKind || isAgentHistoryItem(item) || item.unavailableReason) return false;
  // "unsyncable" (BUG-311) is a permanent fact set by the backend when a chat
  // run has no resumable session file on this machine (e.g. cancelled before
  // the provider ever wrote one) -- it can never later succeed, unlike
  // "failed", which is retried on the next sync attempt.
  if (item.syncStatus === "synced" || item.syncStatus === "unsyncable") return false;
  if (remoteChatSessions && item.sourceMachineId && item.sourceRunId) {
    const alreadySynced = remoteChatSessions.some(
      (remote) => remote.sourceMachineId === item.sourceMachineId && remote.sourceRunId === item.sourceRunId,
    );
    if (alreadySynced) return false;
  }
  return true;
}

export function filterVisibleHistory(history: RunHistoryItem[]): RunHistoryItem[] {
  return history.filter((item) => !isAgentHistoryItem(item));
}

/** Task-455: compact relative timestamp for history rows — "now", "5m",
 *  "2h", "3d", "5mon", "1y". Replaces the long Intl datetime on the
 *  single-line row; rows re-render on every poll so the value stays fresh. */
export function formatRelativeTime(iso: string, nowMs = Date.now()): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "";
  const seconds = Math.max(0, Math.floor((nowMs - then) / 1000));
  if (seconds < 60) return "now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mon`;
  return `${Math.floor(months / 12)}y`;
}

/** Task-455: terminal-state tag rendered inline before the row title —
 *  the compact row has no meta line, so cancelled/failed runs keep a
 *  visible marker. Live states carry icons instead. */
export function historyStatusTag(status: RunHistoryItem["status"]): "Cancelled" | "Failed" | undefined {
  if (status === "cancelled") return "Cancelled";
  if (status === "failed") return "Failed";
  return undefined;
}

function hasBuiltInAgentPromptPrefix(prompt?: string): boolean {
  const normalized = prompt?.trim().toLowerCase() ?? "";
  return (
    normalized.startsWith("you are the coder sub-agent.") ||
    normalized.startsWith("you are the reviewer sub-agent.") ||
    normalized.startsWith("you are the tester sub-agent.")
  );
}
