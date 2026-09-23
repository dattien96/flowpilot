import type { Project, RunHistoryItem } from "@/types/contract";
import type { AttentionItem, AttentionKind } from "./attentionQueue";

// Task-422 (T-1): pure derivation model for the sessions monitor board —
// groups every known run across ALL projects, merges attention waiting-kind,
// marks the focused run. No fetching, no run-state writes: the board is a
// read-only awareness layer over the already-warmed projectHistoryById +
// attention queue slices.

export interface BoardRow {
  runId: string;
  chatId: string;
  projectId: string;
  projectName: string;
  runTitle: string;
  status: string;
  waitingKind?: AttentionKind;
  worktreeBound: boolean;
  updatedAt: string;
  isFocused: boolean;
}

export interface BoardSection {
  projectId: string;
  projectName: string;
  rows: BoardRow[];
}

export function deriveBoardSections(
  historyByProject: Record<string, RunHistoryItem[]>,
  attention: AttentionItem[],
  projects: Project[],
  focusedRunId: string | null | undefined,
): BoardSection[] {
  const waitingByRun = new Map<string, AttentionKind>();
  for (const item of attention) {
    if (!waitingByRun.has(item.runId)) waitingByRun.set(item.runId, item.kind);
  }

  // Registry order — stable, matches Navigator groups. Projects with no
  // loaded history are omitted (empty board sections add noise, not signal).
  const ordered: Array<{ id: string; name: string }> = [];
  const seen = new Set<string>();
  for (const p of projects) {
    if (seen.has(p.id)) continue;
    seen.add(p.id);
    ordered.push({ id: p.id, name: p.name || p.id });
  }
  for (const pid of Object.keys(historyByProject)) {
    if (!seen.has(pid)) ordered.push({ id: pid, name: pid });
  }

  const sections: BoardSection[] = [];
  for (const proj of ordered) {
    const history = historyByProject[proj.id];
    if (!history || history.length === 0) continue;
    const rows: BoardRow[] = history.map((h) => ({
      runId: h.runId,
      chatId: h.chatId || h.runId,
      projectId: h.projectId || proj.id,
      projectName: proj.name,
      runTitle: (h.lastPrompt || h.lastMessage || "").trim() || h.runId,
      status: h.status,
      waitingKind: waitingByRun.get(h.runId),
      worktreeBound: Boolean(h.worktreeState || h.worktreeSlug || h.worktreePath),
      updatedAt: h.updatedAt,
      isFocused: h.runId === focusedRunId,
    }));
    // Most recent first within a section.
    rows.sort((a, b) => (a.updatedAt > b.updatedAt ? -1 : a.updatedAt < b.updatedAt ? 1 : 0));
    sections.push({ projectId: proj.id, projectName: proj.name, rows });
  }
  return sections;
}

// Task-425 (T-2): spectator view — one watched run's read-only projection,
// derived from the same warmed slices as the board (no fetch, no stream).
export interface SpectatorView {
  runId: string;
  chatId: string;
  projectId: string;
  projectName: string;
  runTitle: string;
  status: string;
  waitingKind?: AttentionKind;
  lastLine: string;
  updatedAt: string;
}

export function deriveSpectatorView(
  historyByProject: Record<string, RunHistoryItem[]>,
  attention: AttentionItem[],
  projects: Project[],
  runId: string | null | undefined,
  projectId: string | null | undefined,
): SpectatorView | null {
  if (!runId) return null;
  const item =
    (projectId ? historyByProject[projectId] : undefined)?.find((h) => h.runId === runId) ??
    Object.values(historyByProject).flat().find((h) => h.runId === runId);
  if (!item) return null;
  const proj = projects.find((p) => p.id === item.projectId);
  const waiting = attention.find((a) => a.runId === runId);
  return {
    runId: item.runId,
    chatId: item.chatId || item.runId,
    projectId: item.projectId,
    projectName: proj?.name || item.projectId,
    runTitle: (item.lastPrompt || item.lastMessage || "").trim() || item.runId,
    status: item.status,
    waitingKind: waiting?.kind,
    lastLine: (item.lastMessage || item.lastPrompt || "").trim(),
    updatedAt: item.updatedAt,
  };
}
