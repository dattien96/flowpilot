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

function hasBuiltInAgentPromptPrefix(prompt?: string): boolean {
  const normalized = prompt?.trim().toLowerCase() ?? "";
  return (
    normalized.startsWith("you are the coder sub-agent.") ||
    normalized.startsWith("you are the reviewer sub-agent.") ||
    normalized.startsWith("you are the tester sub-agent.")
  );
}
