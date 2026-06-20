import type { RunHistoryItem } from "@/types/contract";

export function isProjectSyncing(history: RunHistoryItem[], projectId: string): boolean {
  return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}

export function isAgentHistoryItem(item: RunHistoryItem): boolean {
  return Boolean(item.parentRunId);
}

export function filterVisibleHistory(history: RunHistoryItem[]): RunHistoryItem[] {
  return history.filter((item) => !isAgentHistoryItem(item));
}
