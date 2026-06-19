import type { RunHistoryItem } from "@/types/contract";

export function isProjectSyncing(history: RunHistoryItem[], projectId: string): boolean {
  return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}
