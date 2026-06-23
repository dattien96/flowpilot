import type { RunHistoryItem } from "@/types/contract";

export function isProjectSyncing(history: RunHistoryItem[], projectId: string): boolean {
  return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}

export function isAgentHistoryItem(item: RunHistoryItem): boolean {
  return Boolean(item.parentRunId || hasBuiltInAgentPromptPrefix(item.lastPrompt));
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
