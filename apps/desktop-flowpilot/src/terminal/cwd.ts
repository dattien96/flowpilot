import type { Project, RunHistoryItem } from "@/types/contract";

/**
 * Task-428 T-1: cwd resolution for the embedded terminal — the only
 * FlowPilot-aware piece. The focused run's worktreePath (Task-426, populated
 * from RunHandle/RunHistoryItem) wins; otherwise the selected project root.
 * Resolved once at spawn time and pinned per tab.
 */
export function resolveTerminalCwd(state: {
  runId?: string | null;
  activeWorktreePath?: string;
  runHistory?: RunHistoryItem[];
  selectedProjectId?: string | null;
  projects: Project[];
}): string | null {
  const bound = state.activeWorktreePath?.trim();
  if (bound) return bound;
  if (state.runId) {
    const item = state.runHistory?.find((r) => r.runId === state.runId);
    const itemPath = item?.worktreePath?.trim();
    if (itemPath) return itemPath;
  }
  const project = state.projects.find((p) => p.id === state.selectedProjectId);
  const path = project?.path?.trim();
  return path ? path : null;
}
