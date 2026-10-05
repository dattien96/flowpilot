# CA-1213 — Orphan sweep runs on the non-blocked continue path (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04): the spec_align leg (`run-288095`) was
freeze-parked `waiting_user_approval` by `parkFlowForAwaitingUser` after a
gate violation. The parent's escalate card was consumed by a continue —
but the leg stayed parked ~5 minutes until a LATER continue happened to
land while the loop was `blocked`. Leg-level continue/resume endpoints
were no-ops (children own no agent loop).

## Root cause
The parked-orphan sweep (BUG-1185 machinery — redrive children parked
mid-step with owed work) lived only inside `resumeFlowWithFeedback`'s
blocked-resume tail. When the loop was NOT `blocked` — e.g. `cp_lock`
armed ("running") — `wasBlocked=false` early-returned into
`redriveQuietFlowLoop`, which treats a `waiting_user_approval` child as
busy-evidence (`quiet=false`) and no-ops. Net: a card consumed while the
loop showed running stranded every freeze-parked child permanently.

## Fix
- The orphan sweep is extracted to `redriveParkedFlowOrphans`
  (returns `orphanRedrived` + `flushedIntents` — the latter preserves the
  BUG-550 escalate-fallback signal that work was dispatched).
- It now also runs on the `wasBlocked=false` continue path BEFORE the
  quiet-loop redrive.
- New guard: while the parent still owns a live decision surface
  (pendingApprovalID / pending question / resume-confirm / sprint
  boundary) the park belongs to that card — the sweep refuses so
  children stay frozen until the card is answered.

## Tests
- `ca1213_orphan_sweep_running_loop_test.go`
  `TestCA1213_OrphanSweepRevivesParkedChildWithUnfinishedStep` —
  waiting_user_approval child + RUNNING step row → re-driven to running.
  `TestCA1213_OrphanSweepRefusesWhileParentSurfaceLive` — parent
  pendingApprovalID → sweep refuses, child stays parked.
  `TestCA1213_ChildWithOwnCardIsNotOrphan` — child holding its own
  pending approval is not an orphan.
