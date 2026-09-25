# CA-972 — BUG-481: durable worktree resolution phases (cleanup/persist failure)

Date: 2026-09-25
Branch: cp_live_test
Scope: `apps/local-runner/internal/runner`

## Problem

`POST /client/workflow-runs/{runId}/worktree/resolve` reported terminal
success (`merged` / `kept_branch` / `discarded`) even when effects failed:

- `Manager.Cleanup` errors were discarded (`_ = mgr.Cleanup(...)`), so the API
  returned a terminal state while the worktree directory, base sidecar, and
  branch still existed.
- `persistRunWorktreeLocked` was best-effort and returned no error — a failed
  session write produced a run whose RAM said `merged` but whose durable
  record could still say `merge_pending` (split-brain across restart).
- There was no recovery path: after a kill between "patch applied" and
  "state persisted", a retry re-ran `git apply` on an already-landed patch
  (conflict) or re-scanned a removed worktree (fail-closed error), and a
  different mode call raced the half-done effects.

## Fix — durable resolution transaction

`resolveWorktree` now routes the three destructive modes through
`resolveDestructiveWorktree`, a phased transaction with durable intent:

```
resolution_requested → repository_effect_committed → cleanup_committed → finalized (intent cleared)
```

- `ProviderSessionState`/`worktreeBinding`/`worktreeView` carry
  `worktree_resolution_{id,mode,phase}` (`Resolution *worktreeResolution`),
  round-tripped through the NDJSON session record — same row as the binding,
  no parallel store.
- `persistWorktreeResolution` writes the intent under `s.mu` and rolls the
  in-memory binding back on store failure; `persistRunWorktreeLocked` now
  returns the store error.
- `worktreeCleanupEffect` performs + **verifies** the post-conditions the
  terminal state claims: dir gone, sidecar gone, branch present iff
  `keep_branch` (explicit idempotent `git branch -D` on resume for
  non-keep modes — Cleanup only deletes branches it can still read).
- `gitApplyCheckReverse` (`git apply --check --reverse`) distinguishes
  "patch landed before the crash" from a real conflict on resume.
- `finalizeWorktreeResolution` persists the terminal state + clears the
  intent **before** success is reported or `worktree_resolved` is emitted;
  on persist failure RAM is rolled back and no event fires.
- Failure payloads carry `resolutionId` + durable `phase` with typed codes:
  `worktree_resolution_failed` (503 retryable),
  `worktree_resolution_conflict` (409, different mode past
  `resolution_requested`), `worktree_already_resolved` (409, different
  terminal state). Same-mode terminal replays answer idempotently.
- Mode changes are allowed only at `resolution_requested` (no external
  effect committed yet); once a repo/cleanup effect is committed the
  recorded mode is authoritative.
- `worktree_merge_failed` (500) contract preserved for non-conflict apply
  engine failures — the recorded intent still makes a retry converge.
- BUG-472 evidence gate (`worktree_inspection_failed`) and confirm cards
  unchanged: inspection runs before intent mint, failures leave no intent.

## Tests

`bug481_worktree_resolution_phases_test.go` (red before fix):

- `TestBUG481_CleanupFailureDoesNotReportSuccess` — cleanup error in all
  three modes → 503 `worktree_resolution_failed` with phase, no terminal
  state; retry converges after the fault clears.
- `TestBUG481_FinalizePersistFailureResumesAfterRestart` — finalize write
  fails → 503 at `cleanup_committed`; restart, retry, terminal persisted.
- `TestBUG481_ApplyPatchResumeWithLandedRepoEffect` — phase write fails
  after `git apply` landed → retry detects the landed patch via
  reverse-check, replays cleanup, merges.
- `TestBUG481_ConflictingModeRejected` — `discard` while a `keep_branch`
  resolution sits at `cleanup_committed` → 409 `worktree_resolution_conflict`;
  completing the recorded mode converges.
- `TestBUG481_TerminalReplayIdempotent` — same-mode call on a resolved
  binding answers 200 without re-scanning the removed worktree.
- `TestBUG481_RequestedIntentAllowsModeChange` — mode switch at
  `resolution_requested` succeeds.

## Verification

- `go test -count=1 -run 'TestBUG481|TestBUG472|TestBUG473' ./internal/runner/` — pass
- `go test -count=1 -run 'Worktree|worktree' ./internal/runner/ ./internal/worktree/` — pass
- Full suite `go test -count=1 ./...` — see BUG doc / commit body for result.
