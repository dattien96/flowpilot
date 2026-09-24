# Task-435 — Worktree Merge Card: Per-Action Descriptions + Confirm Gate

- Document ID: `Task-435`
- Title: `Worktree Merge Card Descriptions and Confirm Gate`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `CP-71 (run-worktree), CP-84 (attention-queue)`
- Child Documents: ``
- Related Documents: `CA-958 (keep_branch confirm gate), Task-431 (inbox controls)`
- Replaces: ``
- Tags: `desktop-ui, worktree, attention-queue, confirm-ux`

## AI Quick View

### Summary

- The worktree merge inbox card renders three bare buttons
  (`Apply patch / Keep branch / Discard`) with no consequence text — a
  user cannot tell that `keep_branch` drops uncommitted files or that
  `discard` deletes the branch.
- The runner now returns `409` + `requiresConfirm:true` + file lists for
  dirty `keep_branch`/`discard` (CA-958 + SD-27 Q-2), but the desktop
  has no confirm path: `submitAttentionDecision` just `fail(err)`s and
  the user is stuck — there is no way to proceed from the card.

### Current Ask

- Give each worktree action a short consequence description rendered on
  the card.
- Surface `requiresConfirm` 409s as an in-card confirm state (file list +
  Cancel/Proceed) that resends `resolveWorktreeMerge(runId, mode, true)`.

### Key Decisions

- `T-1` Descriptions live in `decisionControlModel` (pure descriptor),
  not the component — testable without DOM, consistent with the file's
  existing pattern.
- `T-2` Confirm state is a queue-level projection (`item.worktreeConfirm`)
  like `acting`/`evicted` — `decisionControlModel` stays pure-on-item and
  emits a `worktree_confirm` model that replaces the mode buttons.
- `T-3` `RunnerApiError` gains `details` (full response body) — the 409
  body carries `requiresConfirm`/`uncommitted`/`untracked` at top level;
  today only `snapshot` is surfaced.
- `T-4` Proceed is explicit: `submitAttentionDecision(..., confirm)` →
  `resolveWorktreeMerge(runId, mode, true)` — no implicit retry magic.

### Constraints

- Additive-only: no existing test edited; `apply_patch` unchanged (it
  preserves data, needs no confirm).
- Fail-closed preserved: a non-confirm 409 (e.g. `worktree_merge_conflict`)
  still surfaces as an error, never auto-retries.

### Open Questions

- ``

### Source Refs

- `CA-958`, `SD-27 Q-2`, `SS-23 R-3`, `Task-431`

## 1. Goal

The worktree merge decision card tells the user what each action does and
lets them explicitly confirm a destructive resolve on a dirty worktree
instead of dead-ending on a 409.

## 2. Parent Links

- coding plan: CP-71 run-worktree
- tech design: SD-27
- system spec: SS-23
- specific upstream ids: Task-431 (per-kind inbox controls), CA-958

## 3. Trigger

Live validation on Windows (CP-71 matrix) exposed that `keep_branch` on a
dirty worktree silently lost uncommitted agent output; the server-side
confirm gate (CA-958) now blocks it with `409 requiresConfirm`, which the
desktop cannot answer. Separately, the card gives users no consequence
text for any of the three modes.

## 4. Exact Change

- `T-1` `ControlChoice.description?: string`; the three worktree modes
  carry consequence text rendered under each button.
- `T-2` `attentionQueue`: `worktreeConfirms` map +
  `setWorktreeConfirm(runId, {mode, modeLabel, files, message})` /
  `clearWorktreeConfirm(runId)`; projected onto `item.worktreeConfirm` at
  recompute; cleared on `evict`.
- `T-3` `RunnerApiError.details?: Record<string, unknown>` — `parse`
  passes the response body so 409 fields reach the store.
- `T-4` `submitAttentionDecision(runId, decision, choice, customText?,
  confirm?)`; worktree_merge catch: `status===409 &&
  details.requiresConfirm===true` → `setWorktreeConfirm` + `return true`
  (awaiting user, not a failure). `dismissWorktreeConfirm(runId)` clears.
- `T-5` `DecisionControls`: `worktree_confirm` model renders message +
  file list + `Cancel`/`Proceed anyway` → `onAct(mode, undefined, true)`;
  `Cancel` → `onDismiss()` → `dismissWorktreeConfirm`.

## 5. Touched Areas

- files: `src/components/DecisionControls.tsx`, `src/state/attentionQueue.ts`,
  `src/state/store.ts`, `src/client/HttpWsRunnerClient.ts`,
  `src/client/MockRunnerClient.ts`, `src/components/AttentionInbox.tsx`
- modules: desktop attention inbox
- routes: `POST /client/workflow-runs/{id}/worktree/resolve` (unchanged)
- tables: —

## 6. Code Guide Signatures

```ts
// src/components/DecisionControls.tsx
export interface ControlChoice { value: string; label: string; description?: string } // T-1
export type ControlModel = ... | { type: "worktree_confirm"; mode: string; modeLabel: string; files: string[]; message: string } // T-5
export function DecisionControls(props: { item; acting; onAct: (choice: string, customText?: string, confirm?: boolean) => void; onDismiss?: () => void; onOpen: () => void }) // T-5
```

```ts
// src/state/attentionQueue.ts
export interface AttentionItem { ...; worktreeConfirm?: { mode: string; modeLabel: string; files: string[]; message: string } } // T-2
attentionQueue.setWorktreeConfirm(runId: string, c: { mode: string; modeLabel: string; files: string[]; message: string }): void // T-2
attentionQueue.clearWorktreeConfirm(runId: string): void // T-2
```

```ts
// src/state/store.ts
submitAttentionDecision(runId: string, decision: DecisionPayload, choice: string, customText?: string, confirm?: boolean): Promise<boolean> // T-4
dismissWorktreeConfirm(runId: string): void // T-4
```

```ts
// src/client/HttpWsRunnerClient.ts
export class RunnerApiError extends Error { readonly details?: Record<string, unknown> } // T-3
```

```ts
// src/client/MockRunnerClient.ts
resolveWorktreeMergeError?: (runId: string, mode: string) => unknown // T-4 test hook
```

## 7. Test Signatures

- `test("worktree modes carry consequence descriptions")` — asserts all
  three modes have non-empty `description` (covers T-1)
- `test("worktree 409 requiresConfirm flips card to confirm state")` —
  mock throws `RunnerApiError(409, "worktree_keep_branch_confirm", ...,
  {requiresConfirm:true, uncommitted:["a.txt"]})` → `submitAttentionDecision`
  returns true, `item.worktreeConfirm` populated with mode+files (covers T-3, T-4)
- `test("worktree confirm resends with confirm:true and evicts")` —
  `submitAttentionDecision(runId, d, "keep_branch", undefined, true)` →
  `resolvedWorktrees` has `confirm:true`, item evicted, confirm cleared
  (covers T-4)
- `test("dismissWorktreeConfirm restores mode buttons")` — confirm state
  cleared → `decisionControlModel` returns `worktree` again (covers T-5)
- `test("worktree non-confirm 409 still fails")` — `worktree_merge_conflict`
  propagates as failure, no confirm state set (covers constraints)

## 8. Acceptance Check

- Dirty keep_branch from the inbox shows the file list and only proceeds
  after Proceed; Cancel restores the three buttons.
- Clean keep_branch/apply_patch unaffected — resolve immediately.

## 9. Out of Scope

- Conflict-card UI for `worktree_merge_conflict` (still an error surface;
  retry is a separate card action).
- TUI parity for the confirm prompt (TUI worktree card has its own path).
- Auto-committing uncommitted work onto the kept branch (server-side
  decision; not a UI task).

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [ ] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: shipped — descriptions on all three modes; 409 requiresConfirm
  becomes an in-card Cancel/Proceed step that resends with confirm:true;
  5 new tests green, all pre-existing inbox tests green. `worktreeConfirm`
  dropped `modeLabel` from the task-doc shape — the label is derived from
  `WORKTREE_MODES` at model time (single source of truth).
- follow-ups: `worktree_merge_conflict` 409 could later render the
  conflictPaths + patchArtifactRef inline instead of a bare error toast;
  TUI worktree card has no equivalent confirm prompt yet.
- upstream docs updated: CA-960
