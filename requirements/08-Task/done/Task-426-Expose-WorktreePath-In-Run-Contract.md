# Task-426 — Expose `worktreePath` in Run Snapshot/History Contract

- Document ID: `Task-426`
- Title: `Expose worktreePath in Run Contract`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-83 (Embedded Terminal)`
- Child Documents: ``
- Related Documents: `SS-23, SD-27, CP-71`
- Replaces: ``
- Tags: `run-worktree, contract, local-runner`

## AI Quick View

### Summary

- The desktop terminal (and future consumers) needs the run's effective
  working directory. `WorktreePath` is already persisted in
  `workflow_store.go` — this task surfaces it read-only on
  `RunHistoryItem`/`RunSnapshot` so clients never reconstruct
  `.flowpilot/worktrees/<ownerID>` client-side.
- Pure additive field; absent for non-worktree runs.

### Current Ask

- Add `worktreePath?: string` to the TS contract types and populate it in the
  runner's history/snapshot mappers from the persisted row.

### Key Decisions

- `T-1` Field is emitted only when the run has a worktree binding —
  absent/undefined for normal runs (no empty-string ambiguity).
- `T-2` Absolute path as persisted; client treats it as opaque (may not exist
  if the tree was GC'd — terminal handles that, not the contract).

### Constraints

- Additive JSON field only — no schema migration, no rename.
- Provider-agnostic by construction (binding is provider-independent).

### Open Questions

- Also expose `worktreeBranch`? Not needed for terminal v1 — skip unless free.

### Source Refs

- `CP-83 P-1`, `SS-23`, `SD-27`, `CP-71`

## 1. Goal

Clients can resolve a run's effective cwd without duplicating runner-internal
path conventions.

## 2. Parent Links

- coding plan: CP-83
- tech design: SD-27
- system spec: SS-23
- specific upstream ids: CP-71

## 3. Trigger

CP-83 terminal cwd resolution must not compute `.flowpilot/worktrees/<id>`
itself — that couples the UI to a path convention the runner owns.

## 4. Exact Change

- `T-1` `types/contract.ts`: `RunHistoryItem.worktreePath?: string`;
  `RunSnapshot.worktreePath?: string` (whichever view the history/snapshot
  endpoints serialize).
- `T-2` Runner: populate from `rows[i].WorktreePath` /
  `wf.WorktreePath` in the history item + run snapshot mappers.
- `T-3` No client consumer yet (Task-428 uses it).

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/types/contract.ts`;
  `apps/local-runner/internal/runner/workflow_store.go` or the
  history/snapshot serialization site (locate the mapper that already emits
  `worktreeSlug`/`worktreeState`)
- modules: runner HTTP serialization + TS contract
- routes: existing `listRunHistory`, run snapshot endpoints — additive field
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/types/contract.ts
export interface RunHistoryItem {
  // ... existing fields
  worktreeState?: string;
  worktreeSlug?: string;
  /** Absolute path of the bound worktree dir — only when a binding exists. */
  worktreePath?: string;
}
```

```go
// apps/local-runner/internal/runner/<history-mapper>.go
// wherever worktreeSlug is emitted today:
//   item["worktreePath"] = row.WorktreePath   // omit when empty
```

## 7. Test Signatures

- `TestRunHistoryItem_IncludesWorktreePathWhenBound` (Go) — bound run serializes path (AC-1)
- `TestRunHistoryItem_OmitsWorktreePathWhenUnbound` (Go) — absent field, not `""` (AC-2)
- `test("contract type accepts worktreePath optional", ...)` — TS compile-level check (AC-3)

## 8. Acceptance Check

- `GET` history for a worktree-bound run returns `worktreePath` matching the
  on-disk `.flowpilot/worktrees/<ownerID>` dir; normal runs omit it.

## 9. Out of Scope

- Terminal implementation itself; worktree GC semantics; merge-back.

## 10. Definition of Done

- [ ] §6 field present in contract + emitted by runner
- [ ] §7 tests green, additive-only; pre-existing serialization tests green (R1)
- [ ] Provider-agnostic evidenced (R2)
- [ ] `feature_key` = `run-worktree`; CA entry
- [ ] `detect_changes` clean

## 11. Completion Notes

- result: implemented — `worktreePath?: string` on RunHistoryItem + RunSnapshot in Go and TS contract; populated at start/resume/history-listing from ProviderSessionState.WorktreePath and run binding.
- follow-ups: none — consumed by Task-428 resolveTerminalCwd.
- upstream docs updated: CA-924.
