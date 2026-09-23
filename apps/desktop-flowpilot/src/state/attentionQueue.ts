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

import type { DecisionPayload, RunHistoryItem, RunRealtimeFrame, RunRealtimeProjection } from "@/types/contract";
import type { PendingApproval, PendingQuestion } from "@/state/timelineReducer";

export type AttentionKind =
  | "approval"
  | "question"
  | "gate"
  | "ss_lock"
  | "cp_lock"
  | "r_requirement"
  | "decision"
  | "worktree_merge"
  | "quota"
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
  /**
   * CP-84 (Task-431): the originating decision payload — drives per-kind
   * inbox controls/previews. Populated from the mux lane; absent for
   * poll-only items (they render Open-only, never guessed).
   */
  decision?: DecisionPayload;
}

/** Minimal pending-fields projection — satisfied by the internal RunSnapshot
 *  cache and by the /client/workflow-runs/{id} snapshot view. */
export interface RunSnapshotAttentionView {
  status?: string;
  pendingApprovals?: PendingApproval[];
  pendingQuestions?: PendingQuestion[];
  dispatchAttention?: Array<{ kind?: string }>;
  /** CP-84 (Task-429): actionable decision kinds from the mux lane projection —
   *  lets a terminal run carrying a pending worktree-merge decision still
   *  surface an inbox item when its status alone maps to no waiting kind. */
  actionableKinds?: AttentionKind[];
  /** CP-84 (Task-431): the most relevant actionable decision payload for
   *  per-kind controls/previews. */
  decision?: DecisionPayload;
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
    const snap = snapshots[h.runId];
    // CP-84: a lane whose status maps to no waiting kind still surfaces when
    // the mux projection carries an actionable decision (e.g. merge_pending
    // worktree on a completed run).
    if (!base && (snap?.actionableKinds?.length ?? 0) === 0) continue;
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
      decision: snap?.decision,
    });
  }
  // Oldest waiting runs first (spec AC).
  items.sort((a, b) => (a.waitingSince < b.waitingSince ? -1 : a.waitingSince > b.waitingSince ? 1 : 0));
  return items;
}

function refineKind(base: AttentionKind | undefined, snap: RunSnapshotAttentionView | undefined): AttentionKind {
  if (!snap) return base ?? "decision";
  if (snap.dispatchAttention && snap.dispatchAttention.length > 0) return "dispatch_attention";
  if (snap.pendingApprovals && snap.pendingApprovals.length > 0) return "approval";
  if (snap.pendingQuestions && snap.pendingQuestions.length > 0) {
    const prompt = (snap.pendingQuestions[0].prompt ?? "").toLowerCase();
    if (prompt.includes("ss-lock") || prompt.includes("ss lock")) return "ss_lock";
    if (prompt.includes("cp-lock") || prompt.includes("cp lock")) return "cp_lock";
    return "question";
  }
  // Task-431: when the lane carries a decision payload its kind is the
  // truthful label — a "waiting_approval" status covering an ss_lock gate
  // must not mislabel the item as a plain approval.
  return snap.decision?.kind ?? base ?? snap.actionableKinds?.[0] ?? "decision";
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
// CP-84 (Task-431): runs with an in-flight inline decision — shared across
// components so an item can't be double-submitted while a request is out.
const actingRuns = new Set<string>();
// CP-84 (Task-434 T-3): synthetic items pushed for non-focused modal-source
// events (quota, gate). Keyed `${runId}:${kind}` — a repeat event updates the
// entry instead of duplicating it. Pruned when the run resolves.
const syntheticItems = new Map<string, AttentionItem>();

// ---- CP-84 (Task-429): mux-owned lane scope -------------------------------
// RunRealtimeProjections pushed over /client/events/stream live in a
// dedicated map — separate from poll-owned historyByProject so a reconnect
// snapshot drops ONLY mux lanes (stale-item cleanup) without touching the
// 30s poll cache, and so a mux lane can freshen a stale polled status.
const muxLanes = new Map<string, RunRealtimeProjection>();
// Last applied projection revision per run — stale/duplicate upserts drop.
const muxRevisions = new Map<string, number>();
// Snapshot staging (T-4): chunks accumulate under one snapshotId and
// reconcile ONLY on complete — a disconnect mid-burst discards the stage.
// Live upsert/remove frames arriving mid-burst are dropped: the completed
// snapshot supersedes them.
let muxStagingId = "";
let muxStagingBuf: RunRealtimeProjection[] = [];
let muxStagingOpen = false;

/** Convert a lane projection into the attention snapshot view: actionable
 *  approval/question decisions become inline-actionable pending payloads;
 *  dispatch decisions become dispatchAttention; everything else contributes
 *  its kind so non-waiting statuses (terminal + merge_pending) still surface. */
function laneAttentionView(p: RunRealtimeProjection): RunSnapshotAttentionView {
  const approvals: PendingApproval[] = [];
  const questions: PendingQuestion[] = [];
  const dispatch: Array<{ kind?: string }> = [];
  const kinds: AttentionKind[] = [];
  // Task-431: prefer the first non-approval/question actionable decision
  // (they surface via `pending` already); fall back to any actionable one.
  let decision: DecisionPayload | undefined;
  let fallback: DecisionPayload | undefined;
  for (const d of p.decisions ?? []) {
    if (d.actionable) {
      if (d.kind !== "approval" && d.kind !== "question" && !decision) decision = d;
      if (!fallback) fallback = d;
    }
    if (d.kind === "approval" && d.actionable) {
      approvals.push({
        approvalId: d.id.replace(/^approval:/, ""),
        details: {
          command: d.approval?.command,
          cwd: d.approval?.cwd,
          reason: d.approval?.reason,
          kind: d.approval?.kind as PendingApproval["details"]["kind"],
          decisions: d.approval?.decisions ?? [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }],
        },
      });
      continue;
    }
    if (d.kind === "question" && d.actionable) {
      questions.push({
        questionId: d.id.replace(/^question:/, ""),
        prompt: d.prompt ?? "",
        options: d.question?.options ?? [],
        multiSelect: d.question?.multiSelect,
      });
      continue;
    }
    if (d.kind === "dispatch_attention") {
      dispatch.push({ kind: d.dispatch?.attentionKind });
      continue;
    }
    if (d.actionable) kinds.push(d.kind as AttentionKind);
  }
  return {
    status: p.status,
    pendingApprovals: approvals,
    pendingQuestions: questions,
    dispatchAttention: dispatch,
    actionableKinds: kinds,
    decision: decision ?? fallback,
  };
}

/** Synthesize the minimal history row for a mux lane with no polled entry —
 *  the lane must surface in the inbox before the next 30s poll lands. */
function laneHistoryRow(p: RunRealtimeProjection): RunHistoryItem {
  return {
    runId: p.runId,
    projectId: p.projectId,
    chatId: p.chatId,
    providerKey: (p.decisions?.[0]?.providerKey as RunHistoryItem["providerKey"]) ?? "claude",
    status: p.status,
    startedAt: p.updatedAt,
    updatedAt: p.updatedAt,
    lastMessage: p.lastSummary,
  };
}

function mergedSnapshots(): Record<string, RunSnapshotAttentionView | undefined> {
  const out: Record<string, RunSnapshotAttentionView | undefined> = { ...snapshotViews };
  for (const [runId, items] of Object.entries(dispatchViews)) {
    out[runId] = { ...(out[runId] ?? {}), dispatchAttention: items };
  }
  // Mux lane views overlay — they carry the freshest decision state.
  for (const [runId, lane] of muxLanes) {
    out[runId] = { ...(out[runId] ?? {}), ...laneAttentionView(lane) };
  }
  return out;
}

function recompute(): void {
  const snapshots = mergedSnapshots();
  const items: AttentionItem[] = [];
  const covered = new Set<string>();
  for (const [projectId, history] of historyByProject) {
    // A mux lane overrides the polled status/updatedAt for the same run —
    // the stream is fresher than the 30s poll (T-6).
    const overlaid = history.map((h) => {
      const lane = muxLanes.get(h.runId);
      if (!lane) return h;
      covered.add(h.runId);
      return { ...h, status: lane.status, updatedAt: lane.updatedAt };
    });
    items.push(...deriveAttentionItems(overlaid, snapshots, projectId, evictedRuns));
  }
  // Mux lanes with no polled history row still surface (new run not yet
  // polled, or terminal+actionable lanes history may have dropped).
  const lanesByProject = new Map<string, RunHistoryItem[]>();
  for (const lane of muxLanes.values()) {
    if (covered.has(lane.runId)) continue;
    const rows = lanesByProject.get(lane.projectId) ?? [];
    rows.push(laneHistoryRow(lane));
    lanesByProject.set(lane.projectId, rows);
  }
  for (const [projectId, rows] of lanesByProject) {
    items.push(...deriveAttentionItems(rows, snapshots, projectId, evictedRuns));
  }
  // Task-434 T-3: synthetic modal-source items — deduped by (runId, kind);
  // dropped once the run's observed status leaves waiting with no actionable
  // decision, or a real (polled/mux) item already covers the same key.
  const seenKeys = new Set(items.map((i) => `${i.runId}:${i.kind}`));
  for (const [key, syn] of syntheticItems) {
    const lane = muxLanes.get(syn.runId);
    const histStale = [...historyByProject.values()]
      .flat()
      .find((h) => h.runId === syn.runId);
    const observed = lane?.status ?? histStale?.status;
    const actionable = (laneAttentionView(lane ?? { runId: syn.runId, projectId: syn.projectId, revision: 0, status: "running", updatedAt: "" }).actionableKinds?.length ?? 0) > 0;
    if (observed && !WAITING_STATUS[observed] && !actionable) {
      syntheticItems.delete(key);
      continue;
    }
    if (!seenKeys.has(key)) items.push(syn);
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
  /** CP-84 (Task-429): atomically replace the mux-owned lane scope on a
   *  completed snapshot burst. */
  reconcileRunSnapshot(runs: RunRealtimeProjection[]): void;
  /** CP-84 (Task-429): revision-guarded upsert / idempotent remove. */
  applyRunUpdate(frame: RunRealtimeFrame): void;
  muxLaneCount(): number;
  /** CP-84 (Task-434 T-3): push a synthetic item for a non-focused
   *  modal-source event; dedupes by (runId, kind). */
  ingestAttentionItem(item: AttentionItem): void;
  /** CP-84 (Task-431): mark/clear an in-flight inline submission for a run. */
  markActing(runId: string): void;
  clearActing(runId: string): void;
  isActing(runId: string): boolean;
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
  ingestAttentionItem(item) {
    syntheticItems.set(`${item.runId}:${item.kind}`, item);
    recompute();
  },
  markActing(runId) {
    actingRuns.add(runId);
    for (const fn of listeners) fn();
  },
  clearActing(runId) {
    actingRuns.delete(runId);
    for (const fn of listeners) fn();
  },
  isActing(runId) {
    return actingRuns.has(runId);
  },
  evict(runId) {
    evictedRuns.add(runId);
    actingRuns.delete(runId);
    for (const key of [...syntheticItems.keys()]) {
      if (key.startsWith(`${runId}:`)) syntheticItems.delete(key);
    }
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
  /**
   * CP-84 (Task-429 T-6): atomically replace the mux-owned lane scope from a
   * COMPLETE snapshot burst. Lanes absent from the snapshot drop out — that
   * is the reconnect path's stale-terminal cleanup. Non-mux state (focused-
   * run snapshot cache, poll history) is untouched.
   */
  reconcileRunSnapshot(runs) {
    muxLanes.clear();
    muxRevisions.clear();
    for (const p of runs) {
      muxLanes.set(p.runId, p);
      muxRevisions.set(p.runId, p.revision);
    }
    recompute();
  },
  /**
   * CP-84 (Task-429 T-4/T-6): apply one mux frame. Snapshot chunks stage
   * under snapshotId and reconcile atomically only on `complete`; frames
   * arriving mid-burst are dropped (the completed snapshot supersedes them).
   * Upserts with a revision at or below the last applied are dropped
   * (dedupe); remove is idempotent; resync is the consumer's cue to
   * reconnect — nothing to apply here.
   */
  applyRunUpdate(frame) {
    if (frame.kind === "snapshot") {
      const id = frame.snapshotId ?? "";
      if (id !== muxStagingId) {
        muxStagingId = id;
        muxStagingBuf = [];
        muxStagingOpen = true;
      }
      muxStagingBuf.push(...(frame.runs ?? []));
      if (frame.complete) {
        muxStagingOpen = false;
        const committed = muxStagingBuf;
        muxStagingBuf = [];
        attentionQueue.reconcileRunSnapshot(committed);
      }
      return;
    }
    if (muxStagingOpen) return;
    if (frame.kind === "upsert" && frame.run) {
      const rev = frame.run.revision;
      if (rev <= (muxRevisions.get(frame.run.runId) ?? -1)) return;
      muxRevisions.set(frame.run.runId, rev);
      muxLanes.set(frame.run.runId, frame.run);
      recompute();
      return;
    }
    if (frame.kind === "remove" && frame.runId) {
      if (!muxLanes.delete(frame.runId)) return;
      muxRevisions.delete(frame.runId);
      recompute();
    }
  },
  /** Test/debug introspection for the mux lane scope. */
  muxLaneCount() {
    return muxLanes.size;
  },
  subscribe(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
  },
};
