# Task-423 — Inbox Inline Attention Actions (No-Focus-Switch Approve/Answer)

- Document ID: `Task-423`
- Title: `Inbox Inline Attention Actions`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-82 (Multi-Project Parallel Vibe Operations)`
- Child Documents: ``
- Related Documents: `Task-404, Task-422, CA-922`
- Replaces: ``
- Tags: `desktop, attention-queue, approvals`

## AI Quick View

### Summary

- Inbox items gain inline actions: Approve/Deny for `approval` kind, option
  chips for `question`/`decision` kinds — executed **without switching
  project or opening the run**.
- Verified feasible: `client.submitApproval(approvalId,…)` and
  `client.answerQuestion(questionId,…)` are ID-scoped RPCs; IDs are globally
  unique server-side. The only unsafe part is the existing `approve()`/
  `answer()` optimistic update which writes the *focused* slot — so this task
  adds run-scoped actions that skip that slot entirely.
- `AttentionItem` gains a `pending` payload (approvals/questions arrays from
  the snapshot) so the inbox can render real options, not just a kind chip.

### Current Ask

- Extend `AttentionItem` + derive payload, add
  `approveAttentionItem`/`answerAttentionItem` store actions, render inline
  controls in `AttentionInbox`, keep "Open" fallback for unactionable kinds.

### Key Decisions

- `T-1` Inline actions never touch `pendingApprovals`/`pendingQuestions`/
  `status`/`timeline` of the focused run — the action is fire-and-forget RPC
  + queue eviction; success is confirmed when the run's next history poll
  shows a non-waiting status.
- `T-2` Optimistic UX is per-item local (busy spinner → resolved flash), not
  global store state. Failure keeps the item and surfaces a toast/error line.
- `T-3` Kinds without a safe inline payload (`gate`, `ss_lock`, `cp_lock`,
  `r_requirement`, `dispatch_attention`, or missing snapshot) render only the
  existing "Open" affordance — never guess.

### Constraints

- Must not regress BUG-157 semantics: approve by `approvalId`, not "the
  pending one". The pending payload on the item comes from the same snapshot
  the kind was refined from — no second source of truth.
- Provider-agnostic: RPC contract is shared; no provider branching.
- Additive tests only; `attention_queue.test.ts` stays untouched.

### Open Questions

- `decision`/`r_requirement` kinds have richer option payloads — inline option
  chips supported only when the snapshot carries `options[]`; otherwise Open.

### Source Refs

- `CP-82 P-2`, `Task-404`, `BUG-157`, `BUG-172`

## 1. Goal

An operator can clear approval/question waits across projects from the inbox
alone — approve a run in project B while still focused on project A.

## 2. Parent Links

- coding plan: CP-82
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-404, BUG-157, BUG-172, CA-922

## 3. Trigger

CP-82 P-2: inbox visibility without action still forces a context switch per
wait; at 8–12 parallel runs that is the dominant friction.

## 4. Exact Change

- `T-1` `attentionQueue.ts`: `AttentionItem` gains
  `pending?: { approvals: PendingApproval[]; questions: PendingQuestion[] }`,
  populated in `deriveAttentionItems` from the same snapshot used for
  `refineKind`. Absent when no snapshot cached.
- `T-2` `store.ts`: two new actions calling the client directly — never
  `get().approve`/`get().answer`.
- `T-3` `attentionQueue`: new `evict(runId)` removing the run's item +
  listener fire (optimistic removal on action success).
- `T-4` `AttentionInbox.tsx`: per-item inline controls — Approve/Deny buttons
  (`approval`), option chips (`question`/`decision` with `options`),
  busy/disabled state per item, error line on rejection; "Open" always
  present.
- `T-5` Styles for `.inbox-actions`, `.inbox-act-*` chips — tokens only.

## 5. Touched Areas

- files: `src/state/attentionQueue.ts`, `src/state/store.ts`,
  `src/components/AttentionInbox.tsx`, `src/styles.css`,
  `src/state/attentionQueue.inline.test.ts` (new),
  `src/state/store.attention-actions.test.ts` (new)
- modules: desktop renderer
- routes: none (existing `/client/approvals/{id}`, `/client/questions/{id}`)
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts
export interface AttentionItem {
  runId: string; chatId: string; projectId: string; runTitle: string;
  kind: AttentionKind; waitingSince: string; providerKey?: string;
  /** Pending payloads from the run snapshot — drives inline actions (T-1). */
  pending?: { approvals: PendingApproval[]; questions: PendingQuestion[] };
}
export const attentionQueue: {
  items: AttentionItem[];
  ingestHistory(items: RunHistoryItem[], projectId?: string): void;
  ingestSnapshot(runId: string, snap: RunSnapshotAttentionView): void;
  ingestDispatch(runId: string, items: Array<{ kind?: string }>): void;
  evict(runId: string): void;            // T-3
  subscribe(fn: Listener): () => void;
}
```

```ts
// apps/desktop-flowpilot/src/state/store.ts
/** Act on a NON-focused run: direct client RPC + queue evict; never writes
 *  focused pendingApprovals/status/timeline. Throws are caught internally and
 *  surfaced via toast; returns false on failure. */
approveAttentionItem(runId: string, approvalId: string, decision: "approved" | "denied", remember?: boolean): Promise<boolean>;
answerAttentionItem(runId: string, questionId: string, choice: string | string[]): Promise<boolean>;
```

```tsx
// apps/desktop-flowpilot/src/components/AttentionInbox.tsx
// per-item: kind approval → [Approve][Deny]; question w/ options → chips;
// busy item id in local useState<Set<string>>; error → inline text + keep item.
```

## 7. Test Signatures

- `test("deriveAttentionItems attaches pending approvals/questions from snapshot", ...)` — T-1/AC-1
- `test("approveAttentionItem calls client.submitApproval and evicts item", ...)` — T-2/T-3/AC-2
- `test("approveAttentionItem does not touch focused pendingApprovals or status", ...)` — R1-critical invariant (AC-3)
- `test("answerAttentionItem posts choice and evicts item", ...)` — AC-2
- `test("failed attention action keeps item and reports error", ...)` — T-2/AC-4
- `test("item without snapshot pending renders Open-only (no inline controls)", ...)` — T-3/AC-5
- `test("approving one of two pending approvals keeps run item until all resolved", ...)` — near-miss BUG-157 shape (AC-6)

## 8. Acceptance Check

- Approving a waiting run in a non-focused project from the inbox clears the
  badge without switching the workspace; the focused run's timeline/status is
  byte-identical before and after.
- Two pending approvals on one run: resolving the first keeps the item (still
  waiting on the second); resolving the last clears it.
- Server 409/error → item stays, error shown, no focus change.

## 9. Out of Scope

- Cross-project "prompt thêm" (quick composer) — deferred per CP-82 decision.
- Dispatch-attention card actions (open-only kind).
- Board-row actions (Task-422 scope creep).

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (R1)
- [ ] Provider parity: shared RPC contract → provider-agnostic, evidence in CA (R2)
- [ ] `feature_key` = `attention-queue`; CA ledger entry written
- [ ] §8 acceptance checks verified
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: implemented — pending payload on AttentionItem, resolvedPending/evict on the queue with auto-clearing suppression, approveAttentionItem/answerAttentionItem run-scoped actions, inline chips in the inbox. Deviation: approveAttentionItem takes `decision: string` (not the 'approved'|'denied' union) to pass through server-provided decision values like approve_for_session. 10 tests green.
- follow-ups: quick-prompt composer deferred per CP-82 option A.
- upstream docs updated: CA-925.
