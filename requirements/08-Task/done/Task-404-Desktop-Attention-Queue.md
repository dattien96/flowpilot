# Task-404: Desktop Attention Queue (Cross-Run Pending-Action Inbox)

## Metadata

- Document ID: `Task-404`
- Title: `Desktop Attention Queue — Aggregated Pending Questions/Approvals/Locks Across Runs`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [SS-23: Run Isolation Worktree](../../05-System-Specs/SS-23-Run-Isolation-Worktree.md), [CP-59: Chat SSOT](../../07-Coding-Plan/done/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md)
- Replaces: `None`
- Tags: `desktop, ux, attention-queue, vibe-mode, multi-run`
- Feature Keys: `attention-queue`

## AI Quick View

### Summary

- With multiple vibe runs executing concurrently, each run's pending user actions (`user.confirm` locks, `r-requirement` cards, decision cards, approvals, dispatch-attention items) are invisible unless that run is focused — the desktop only streams the active run (`shouldApplyRunEvent` drops others).
- Add an **Attention Queue**: a Navigator-level inbox aggregating every run currently waiting on the user, so multi-run vibe work has a single place that answers "what needs me right now".
- **Zero backend/core change**: data comes from existing surfaces — `GET /client/projects/{projectId}/workflow-runs` items already carry `status` (`waiting_user_approval`) and `lastPrompt`; lazy `GET /client/workflow-runs/{runId}` snapshot exposes `pendingApproval`/`pendingQuestion`/`pendingGate`; `GET .../dispatch-attention` already exists per run.

### Current Ask

- Implement the queue UI + its client-side aggregation on desktop: list pending items across all project runs, badge counts, click-to-focus the owning chat/run at the blocking card, live refresh on poll interval and on stream events of the focused run.

### Key Decisions

- `T-1` Queue is **client-side aggregation only** — no new runner endpoint in this task. Source = `runHistory` items with attention statuses + per-run snapshot fetch on expansion/refresh. (If polling cost becomes an issue, a dedicated aggregate endpoint is a follow-up task, not this one.)
- `T-1b` Aggregation lives in a **single app-level observer store** (`state/attentionQueue.ts`, a zustand slice or module singleton): every source (run-history poll, focused-run stream events, per-run snapshot fetches) pushes normalized pending items into it; components only read the derived queue. One owner, no per-component fan-out.
- `T-2` Item model: `{runId, chatId, runTitle, kind ∈ {approval, question, gate, ss_lock, cp_lock, r_requirement, decision, dispatch_attention}, age, providerKey}`; clicking navigates to that chat/run and scrolls to the blocking card.
- `T-3` Placement: collapsible section pinned at top of `Navigator` run list (above history groups), with a global badge count; empty state hidden (zero noise when nothing waits). Scope = **active project only** (matches Navigator scoping).

### Constraints

- **Must not touch core logic**: no changes to `internal/runner`, flow engine, gate evaluation, or provider code — desktop client + UI only.
- Additive tests only (`*.test.ts` new files); old vitest suite untouched and green (`safe-fix-contract` R1/R3).
- Must not break `store.chatSwitch`/`chatOpenTimeline` behaviors; reuse existing focus/navigation path used by history picker.
- Respect `RunStatus` enum: never invent a "blocked" status (Navigator already documents the real enum has none — reuse `waiting_user_approval` + snapshot pending fields).

### Open Questions

- `Q-1` Resolved — refresh = singleton observer fed by the existing run-history poll + focused-run stream events (piggyback, ~5–10s while any run is `running`/`waiting_*`); no new SSE channels.
- `Q-2` Resolved — active project only, matching Navigator scoping.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` (`runHistory`, `shouldApplyRunEvent`, `_runReplaySeq`), `src/components/Navigator.tsx` (`runHistory`, `groupRunsByChatId`, status chips), `src/components/{ApprovalCard,QuestionCard,DecisionCard,DispatchAttentionCard,FlowAwaitingUserCard}.tsx` (existing card kinds to aggregate), `src/client/HttpWsRunnerClient.ts` (`streamRun`, `listDispatchAttention`).
- Runner contract: `runHistoryItem` (`interactive_handlers.go:1122`), `runSnapshotView` pending fields (`interactive_handlers.go:1112`), `GET .../dispatch-attention` (`dispatch_operator.go:51`).

## 1. Goal

Give the desktop a single, always-visible inbox of every run waiting for user input, so N concurrent vibe flows are manageable without hunting through chats.

## 2. Parent Links

- coding plan: `None (standalone UX task)`
- tech design: `SD-24` (vibe UX surface)
- system spec: `SS-18` (vibe mode — the multi-run driver)
- specific upstream ids: `CP-59` (chat SSOT grouping), `SS-23` (sibling multi-run work)

## 3. Trigger

Vibe mode makes parallel runs the intended usage; today pending questions on unfocused runs are invisible because only the focused run streams events. This task closes the discoverability gap with pure client-side aggregation.

## 4. Exact Change

- `T-1` New `AttentionQueue.tsx` component + `state/attentionQueue.ts` **singleton observer store** deriving items from `runHistory` (statuses `waiting_user_approval`, `blocked`-equivalent pending states) and enriching the top item per run via `runSnapshotView` pending fields; all producers push into this one store.
- `T-2` Mount queue at top of `Navigator` (collapsible, badge count, per-kind icon/color reusing existing card semantics); click → existing open-run/attach path → focus blocking card.
- `T-3` Refresh triggers: run-history poll, focused-run stream events, window refocus. No new SSE channels.
- `T-4` New vitest files covering: derivation matrix (each pending kind → one item), ordering (oldest waiting first), empty state, click-navigation payload, multiple pending in same run collapse to one item.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/AttentionQueue.tsx` (new), `src/state/attentionQueue.ts` (new), `Navigator.tsx` (mount), `store.ts` (selector wiring, additive), `HttpWsRunnerClient.ts` (reuse, no new methods expected), `styles.css` (queue styles).
- modules: desktop UI layer only.
- routes: none.
- tables: none.

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts — singleton observer store
export type AttentionKind =
  | "approval" | "question" | "gate"
  | "ss_lock" | "cp_lock" | "r_requirement"
  | "decision" | "dispatch_attention"; // T-2

export interface AttentionItem {
  runId: string;
  chatId: string;
  runTitle: string;
  kind: AttentionKind;
  waitingSince: string;       // ISO — for age display + ordering
  providerKey?: string;
}

// Derivation: runHistory items with waiting statuses → one item per run,
// enriched by runSnapshotView pending fields when fetched.
export function deriveAttentionItems(
  history: RunHistoryItem[],
  snapshots: Record<string, RunSnapshotView | undefined>,
  activeProjectId: string,
): AttentionItem[] // T-1

// Singleton observer: producers (history poll, focused-run events, snapshot
// fetches) push raw signals; consumers read derived items. T-1b
export const attentionQueue: {
  items: AttentionItem[];
  ingestHistory(items: RunHistoryItem[]): void;
  ingestSnapshot(runId: string, snap: RunSnapshotView): void;
  subscribe(fn: () => void): () => void;
}
```

```ts
// apps/desktop-flowpilot/src/components/AttentionQueue.tsx
export function AttentionQueue(props: {
  onOpenRun: (runId: string, chatId: string) => void; // T-2 click → focus
}): JSX.Element | null // null when empty — T-3 zero noise
```

```ts
// apps/desktop-flowpilot/src/state/store.ts — additive wiring only
attentionItems: AttentionItem[]            // derived, subscribed from attentionQueue
openRunAtAttention(runId: string, chatId: string): Promise<void> // T-2
// HttpWsRunnerClient.ts — unchanged (existing listDispatchAttention + snapshot fetch)
```

## 7. Test Signatures

- `test("deriveAttentionItems emits one item per waiting run")` — 3 runs (ss_lock, waiting_approval, running) → exactly 2 items (covers AC core).
- `test("deriveAttentionItems maps every pending kind to AttentionKind")` — snapshot `pendingApproval`/`pendingQuestion`/`pendingGate` + vibe kinds → correct kind labels.
- `test("deriveAttentionItems orders oldest waiting first")` — age ordering invariant.
- `test("deriveAttentionItems scopes to active project only")` — other-project runs excluded (Q-2).
- `test("attentionQueue singleton collapses multiple pending in one run")` — 2 pendings same run → 1 item.
- `test("openRunAtAttention reuses the history-picker open path")` — asserts the same store action is invoked (no parallel navigation).
- `test("AttentionQueue renders null when empty and badge count when non-empty")`.

## 8. Acceptance Check

- A project with 3 runs where run A waits on `ss_lock`, run B on `r-requirement`, run C running: queue shows exactly 2 items, correct kind labels, ordered by wait age; clicking A's item opens chat A at the lock card.
- Queue updates when the run-history poll delivers a status change, without restarting or refocusing.
- No `internal/` (Go) changes in the diff.

## 9. Out of Scope

- Runner-side aggregate endpoint; Admin Web; TUI (product decision: TUI stays single-flow, no indicator); notification/toast redesign; worktree badges (CP-71).

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11).
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited).
- [ ] Pre-existing desktop vitest suite untouched and green; any old failure → STOP + report.
- [ ] Provider parity: N/A by evidence — queue derives from provider-neutral `runHistoryItem`/`runSnapshotView` fields (state evidence recorded in CA note).
- [ ] Queue items aggregate only active-project runs and navigate via the existing open-run path.
- [ ] `feature_key: attention-queue` used; CA ledger entry written.
- [ ] GitNexus `detect_changes` shows only expected desktop files before commit.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
