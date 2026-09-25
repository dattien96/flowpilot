# CA-971 — BUG-473: boot worktree GC requires a proven-orphan verdict

- type: bugfix
- bug: BUG-473 (code-confirmed; live fault-injection pending)
- feature: run-worktree
- follows: CA-970

## Change

`internal/runner/run_worktree_merge.go` `sweepOrphanedWorktrees`:

- The boot GC previously treated persistence read errors as "no binding":
  `ListProviderSessionsByChat`/`GetProviderSession` failures left `bound`
  false and the owner was pruned with `git worktree remove --force` —
  "authority unavailable" became "safe to delete".
- New `worktreeGCVerdictFor` resolves each owner to a three-state verdict:
  - `gcBound` — a matching session row with a live lifecycle state
    (active/merge_pending/resumable/lost/…) exists; never prune.
  - `gcOrphan` — every consulted authority proved no binding; prune.
  - `gcUnknown` — any read error, a matching row with zero/corrupt state,
    or a store that cannot answer a binding class; deferred with a
    `gc_deferred` diagnostic, never deleted.
- `discarded`/`merged` rows still do not protect the dir; a chat lookup
  failure is never re-read through the run lookup as "not found".
- Removal integrity: `removeWorktreeDir` checks `git worktree remove` /
  `RemoveAll` errors; `pruned orphan worktree` is logged only after the dir
  is actually gone — failures log `gc_failed` and skip the owner.
- Candidate (`candidate-` prefix) exclusion and live-binding `s.runs`
  exclusion unchanged. Multi-project scope (sweep covers only the attached
  runner workspace, under-pruning other repos — safe direction) recorded
  on the BUG doc as a separate follow-up.

## Tests (additive only)

- `bug473_worktree_gc_authority_failure_test.go`:
  - `TestBUG473_ChatBindingSurvivesStoreReadFailure` — RED first (pre-fix:
    injected chat-lookup error produced `pruned orphan worktree` and the
    dir was deleted); post-fix: dir + files + base sidecar survive,
    `gc_deferred` logged, no `pruned` line for that owner.
  - `TestBUG473_FlowBindingSurvivesStoreReadFailure` — same contract for
    `GetProviderSession` failure on a run-owned binding.
  - `TestBUG473_CorruptBindingRowDefersGC` — matching row with zero
    `WorktreeState` defers, never prunes.
  - `TestBUG473_SweepRecoversAndPrunesTrueOrphan` — after the store heals,
    the bound worktree still survives while a real orphan is pruned and
    logged.

## Provider parity

Provider-agnostic — boot GC and the session-store boundary are
provider-free; verified via codex/claude-labelled runs interchangeably.

## Invariant

SD-27 §8 / AGENTS durability: GC deletes only on positive proof of
orphan-hood. Non-terminal and unverifiable bindings are never pruned.
