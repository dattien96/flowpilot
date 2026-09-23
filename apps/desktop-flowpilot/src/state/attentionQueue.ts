// Task-404 (T-1/T-1b): client-side attention queue — a singleton observer that
// aggregates pending user actions across all runs of the ACTIVE project into a
// flat, ordered list for the Navigator. Pure derivation: no new backend calls,
// fed by run-history polling (ingestHistory), focused-run snapshot caching
// (ingestSnapshot), and the dispatch-attention card (ingestDispatch).
//
// Inbox update: the queue keeps one history slice per project so the header
// inbox can surface waiting runs across ALL loaded projects, not just the
// selected one — a run stuck in project B stays visible while project A is
// open. Each item carries its projectId so the inbox can switch projects on
// open.

import type { RunHistoryItem } from "@/types/contract";
import type { PendingApproval, PendingQuestion } from "@/state/timelineReducer";

export type AttentionKind =
  | "approval"
  | "question"
  | "gate"
  | "ss_lock"
  | "cp_lock"
  | "r_requirement"
  | "decision"
  | "dispatch_attention";

export interface AttentionItem {
  runId: string;
  chatId: string;
  /** Owning project — the inbox groups/labels by it and switches to it on open. */
  projectId: string;
  runTitle: string;
  kind: AttentionKind;
  waitingSince: string;
  providerKey?: string;
  /**
   * Task-423 T-1: pending payloads copied from the same snapshot that refined
   * `kind` — drives inline inbox actions. Absent when no snapshot is cached;
   * the inbox renders Open-only then (never guesses).
   */
  pending?: { approvals: PendingApproval[]; questions: PendingQuestion[] };
}

/** Minimal pending-fields projection — satisfied by the internal RunSnapshot
 *  cache and by the /client/workflow-runs/{id} snapshot view. */
export interface RunSnapshotAttentionView {
  status?: string;
  pendingApprovals?: PendingApproval[];
  pendingQuestions?: PendingQuestion[];
  dispatchAttention?: Array<{ kind?: string }>;
}

// RunStatus strings that mean "a human must act before this run can proceed".
const WAITING_STATUS: Record<string, AttentionKind> = {
  waiting_approval: "approval",
  waiting_user_approval: "approval",
  waiting_user_confirm: "ss_lock",
  waiting_question: "question",
  blocked: "gate",
};

export function deriveAttentionItems(
  history: RunHistoryItem[],
  snapshots: Record<string, RunSnapshotAttentionView | undefined>,
  activeProjectId: string,
  evicted?: Set<string>,
): AttentionItem[] {
  const items: AttentionItem[] = [];
  for (const h of history) {
    if (h.projectId !== activeProjectId) continue;
    const base = WAITING_STATUS[h.status];
    if (evicted?.has(h.runId)) {
      // Task-423 T-1: optimistically resolved inline — stay suppressed while
      // the server still reports a waiting status; clear once the run's
      // observed status leaves waiting so a genuinely NEW wait re-surfaces.
      if (base) continue;
      evicted.delete(h.runId);
    }
    if (!base) continue;
    const snap = snapshots[h.runId];
    const pending =
      snap && ((snap.pendingApprovals?.length ?? 0) > 0 || (snap.pendingQuestions?.length ?? 0) > 0)
        ? { approvals: snap.pendingApprovals ?? [], questions: snap.pendingQuestions ?? [] }
        : undefined;
    items.push({
      runId: h.runId,
      chatId: h.chatId || h.runId,
      projectId: h.projectId,
      runTitle: (h.lastPrompt || h.lastMessage || "").trim() || h.runId,
      kind: refineKind(base, snap),
      waitingSince: h.updatedAt,
      providerKey: h.providerKey,
      pending,
    });
  }
  // Oldest waiting runs first (spec AC).
  items.sort((a, b) => (a.waitingSince < b.waitingSince ? -1 : a.waitingSince > b.waitingSince ? 1 : 0));
  return items;
}

function refineKind(base: AttentionKind, snap: RunSnapshotAttentionView | undefined): AttentionKind {
  if (!snap) return base;
  if (snap.dispatchAttention && snap.dispatchAttention.length > 0) return "dispatch_attention";
  if (snap.pendingApprovals && snap.pendingApprovals.length > 0) return "approval";
  if (snap.pendingQuestions && snap.pendingQuestions.length > 0) {
    const prompt = (snap.pendingQuestions[0].prompt ?? "").toLowerCase();
    if (prompt.includes("ss-lock") || prompt.includes("ss lock")) return "ss_lock";
    if (prompt.includes("cp-lock") || prompt.includes("cp lock")) return "cp_lock";
    return "question";
  }
  return base;
}

type Listener = () => void;

const listeners = new Set<Listener>();
// Per-project history slices — a project's entries stay cached (and keep
// contributing attention items) while the user browses another project.
const historyByProject = new Map<string, RunHistoryItem[]>();
const snapshotViews: Record<string, RunSnapshotAttentionView | undefined> = {};
const dispatchViews: Record<string, Array<{ kind?: string }>> = {};
// Task-423 T-3: runs resolved via inline actions stay hidden until the server
// reports a non-waiting status (cleared inside deriveAttentionItems).
const evictedRuns = new Set<string>();

function mergedSnapshots(): Record<string, RunSnapshotAttentionView | undefined> {
  const out: Record<string, RunSnapshotAttentionView | undefined> = { ...snapshotViews };
  for (const [runId, items] of Object.entries(dispatchViews)) {
    out[runId] = { ...(out[runId] ?? {}), dispatchAttention: items };
  }
  return out;
}

function recompute(): void {
  const snapshots = mergedSnapshots();
  const items: AttentionItem[] = [];
  for (const [projectId, history] of historyByProject) {
    items.push(...deriveAttentionItems(history, snapshots, projectId, evictedRuns));
  }
  // Oldest waiting runs first (spec AC) — across all projects.
  items.sort((a, b) => (a.waitingSince < b.waitingSince ? -1 : a.waitingSince > b.waitingSince ? 1 : 0));
  attentionQueue.items = items;
  for (const fn of listeners) fn();
}

export const attentionQueue: {
  items: AttentionItem[];
  ingestHistory(items: RunHistoryItem[], projectId?: string): void;
  ingestSnapshot(runId: string, snap: RunSnapshotAttentionView): void;
  ingestDispatch(runId: string, items: Array<{ kind?: string }>): void;
  evict(runId: string): void;
  resolvedPending(runId: string, resolved: { approvalId?: string; questionId?: string }): void;
  subscribe(fn: Listener): () => void;
} = {
  items: [],
  ingestHistory(items, projectId) {
    if (projectId !== undefined) {
      historyByProject.set(projectId, items);
    } else {
      // No owning project supplied — rebuild every slice from the items' own
      // projectId fields (matches the pre-slice "replace everything" semantics).
      historyByProject.clear();
      for (const item of items) {
        const slice = historyByProject.get(item.projectId);
        if (slice) slice.push(item);
        else historyByProject.set(item.projectId, [item]);
      }
    }
    recompute();
  },
  ingestSnapshot(runId, snap) {
    snapshotViews[runId] = snap;
    recompute();
  },
  /** Task-423 T-3: hide a run's item optimistically after the last inline
   *  action succeeded. Suppression auto-clears when the run's next observed
   *  history status is non-waiting. */
  evict(runId) {
    evictedRuns.add(runId);
    recompute();
  },
  /** Task-423 T-1: drop a resolved approval/question from the cached snapshot
   *  so a run that still has OTHER pending items keeps its inbox row with a
   *  truthful payload (BUG-157 shape — several approvals can be outstanding). */
  resolvedPending(runId, resolved) {
    const snap = snapshotViews[runId];
    if (snap) {
      snapshotViews[runId] = {
        ...snap,
        pendingApprovals: resolved.approvalId
          ? (snap.pendingApprovals ?? []).filter((a) => a.approvalId !== resolved.approvalId)
          : snap.pendingApprovals,
        pendingQuestions: resolved.questionId
          ? (snap.pendingQuestions ?? []).filter((q) => q.questionId !== resolved.questionId)
          : snap.pendingQuestions,
      };
    }
    recompute();
  },
  ingestDispatch(runId, items) {
    if (items.length === 0) delete dispatchViews[runId];
    else dispatchViews[runId] = items;
    recompute();
  },
  subscribe(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
  },
};
