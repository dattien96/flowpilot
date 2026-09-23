# CA-928 — Desktop: worktree toggle silently disarmed by New run

## Summary

Manual CP-83 M-3 verification: user enabled the Worktree toggle, clicked
"New run", and sent a prompt — the toggle showed Off and the run executed in
the project root (`cwd="D:\working\gate-sandbox"` in runner logs, no
worktree path, no `worktree:true` in the request).

## Root cause

`resetRun()` cleared `worktreeEnabled: false`. The header "New run" action
calls `resetRun()`, so arming the toggle and then starting a new run wiped
the intent before `sendPrompt()` built the request. The toggle is a
next-run intent — same class as `yoloMode`/`workingMode`, which `resetRun`
already preserves.

## Fix

- `resetRun()` no longer touches `worktreeEnabled` — the armed intent
  survives "New run" / new-chat.
- `selectProject()` explicitly clears `worktreeEnabled` when the project
  actually changes — the intent stays project-scoped so a stale `true` can
  never send `worktree:true` to a non-git project. Runner remains the
  authority on availability via `refreshWorktreeAvailability()`.

Per-run state (`activeWorktreePath`, `activeWorktreeState`) still resets —
those describe the previous run's binding, not intent.

## Tests (additive)

`src/state/worktree_intent.test.ts`:
- intent survives `resetRun()`
- switching projects clears it; re-selecting the same project does not
- end-to-end: toggle ON → `resetRun()` → `sendPrompt()` → `startRun`
  payload carries `worktree:true`, returned `worktreePath` lands on store

Existing `cp71_worktree_toggle.test.ts` unmodified and passing.

## Files

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/worktree_intent.test.ts` (new)

# ---8<--- flowpilot:change-ledger
feature_key: run-worktree
source_doc_id: CP-83
change_type: bugfix
summary: resetRun no longer disarms worktree intent; selectProject clears it on real project change
# --->8---
