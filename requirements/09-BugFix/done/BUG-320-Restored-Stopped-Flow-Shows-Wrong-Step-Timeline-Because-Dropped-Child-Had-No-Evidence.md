# BUG-320: Restored Stopped Flow Shows Wrong Step Timeline Because a Dropped Child Had No Evidence

## Metadata

- Document ID: `BUG-320`
- Title: `resumedFlowStepRows defaults every evidence-less flow step to DONE whenever the run's own last chat turn completed, even when its LoopState is "stopped" -- so a restored (or same-machine-restarted) flow whose reviewer never finished and whose synthesis node never ran shows both as DONE ("Round 1/3, 3/3 steps") instead of the truth (reviewer FAILED, synthesis PENDING)`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-07-24`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: [BUG-319](../done/BUG-319-Restore-Aborts-Whole-Chat-Tree-When-One-Child-Was-Never-Synced.md) (this fix's tombstone record is the evidence the step-timeline fix below needs to see), [BUG-260](../done/BUG-260-Resumed-Flow-Fast-Path-Overwrites-Failed-Cohort-Member-As-Done.md) (a failed cohort member must survive a genuinely done flow -- this fix must not regress that), [BUG-308](../done/BUG-308-Stopped-Run-Sealed-And-Prompt-Vanishes.md) (a stopped loop's own chat-turn history badge intentionally still reads "Completed" -- this fix does NOT change that, only what the flow's own steps show), [CA-419](../../change-audit/CA-419-stopped-flow-step-timeline-respects-tombstone-evidence.md)
- Replaces: `none`
- Tags: `agent-flow-engine, google-drive, flow-step-timeline, restore, resume, cross-provider, severity-medium`

## AI Quick View

### Summary

Live-verified immediately after BUG-319 shipped: the user reproduced a Claude
flow-claude chat stopped mid-reviewer-turn, synced it, deleted it locally,
restored it from Drive, and opened it. The restore itself succeeded (BUG-319),
but the step-timeline panel showed "Round 1/3, 3/3 steps" with
`my-reviewer-claude` and `synthesis` both DONE -- even though the source
machine's own flow diagnostic log
(`.flowpilot/logs/features/agent-flow-engine/run-55348.ndjson`) proved the
reviewer step actually went `RUNNING -> CANCELED -> FAILED` and `synthesis`
never transitioned at all (the flow was stopped before it ever reached
synthesis).

### Current Ask

Make `resumedFlowStepRows`'s "no evidence -> default to DONE" rule -- and the
transition-log replay branch's separate hub-promotion check -- both agree that
a **stopped** loop's evidence-less steps stay PENDING, not DONE. A genuinely
**done** loop must keep defaulting evidence-less steps to DONE (BUG-260
parity), and a stopped loop's own chat-turn history badge must keep reading
"Completed" (BUG-308) -- only the FLOW STEP timeline's default changes.

### Key Decisions

- New `resumedFlowStepsComplete(st)` replaces the ad-hoc
  `!resumedFlowRunIncomplete(st) && LoopState.Status != "blocked"` expression,
  adding one more exclusion: `LoopState.Status == "stopped"` is never
  "complete" for this purpose. Used by BOTH the evidence-walk default AND the
  transition-log replay's hub-promotion check, so a Drive restore (no local
  sidecar) and a same-machine restart (sidecar present) of the identical run
  agree on the answer -- before this fix they disagreed (the replay branch
  used the unrelated `normalizeResumedFlowStatus(st) == RunStatusCompleted`
  check, which intentionally returns Completed for a stopped loop with a later
  plain-chat follow-up per BUG-308).
- Deliberately NOT touching `normalizeResumedFlowStatus` or
  `terminalFlowLoopStatus` themselves -- those still correctly answer "what
  should the run's own history badge show" (BUG-308's own contract) and "is
  this loop status terminal for resume-cancel purposes", which are different
  questions from "did the flow's own steps really finish".
- Per-child EVIDENCE (a real session, or BUG-319's tombstone) always wins over
  the default regardless of `resumedFlowStepsComplete` -- unchanged from
  BUG-260's own guarantee. This fix only changes what happens to a node with
  NO evidence at all.
- This bug was only reachable at all because of BUG-319: before that fix, a
  never-synced child hard-failed the ENTIRE restore, so nobody could ever open
  a restored stopped flow to see this defaulting bug in the first place.

### Constraints

- Additive tests only; no pre-existing test edited.
- No real machine paths in tests.

### Open Questions

- `none`.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go` --
  `resumedFlowStepsComplete` (new), `resumedFlowStepRows`'s `flowComplete`
  assignment, the transition-log replay's hub-promotion `if` check.

## 1. Issue Summary

Live repro (same chat as BUG-319): hub `run-55348` (Claude, custom flow
`flow-claude`: `my-coder` -> `my-reviewer-claude` -> `synthesis`), coder
`run-55353` (completed for real), reviewer `run-55467` (Claude MCP connection
timeout, `turn_failed`, 0 events -- user then stopped the flow). After BUG-319
let this restore succeed, opening the restored chat showed the step-timeline
panel as "Round 1/3, 3/3 steps" -- `my-coder` DONE (correct), but
`my-reviewer-claude` ALSO DONE (wrong -- it FAILED) and `synthesis` ALSO DONE
(wrong -- it never ran). The source machine's flow diagnostic log confirms the
true sequence: `my-reviewer-claude` went `RUNNING -> CANCELED -> CANCELED ->
FAILED`; `synthesis` has no transitions logged at all.

## 2. Parent Links

`agent-flow-engine` (step-timeline reconstruction) and `google-drive`
(reachable only via a BUG-319-restored run, though the SAME defaulting bug
also affects a same-machine restart of a stopped flow with a local
transition-log sidecar -- see the transition-log-replay test below).

## 3. Environment and Reproduction

Runner started with `just dev`; project Gate-sandbox. Custom flow
`flow-claude`.

1. Start a flow-claude review-loop chat; stop it while the reviewer child's
   turn is still starting (before it produces any real output) -- BUG-319's
   exact repro.
2. Sync up, delete locally, restore from Drive (now succeeds per BUG-319).
3. Open the restored chat.
4. The step-timeline panel shows the reviewer and synthesis as DONE. They
   should show FAILED and PENDING respectively.

## 4. Expected vs Actual

- Expected: a restored (or same-machine-restarted) stopped flow's step
  timeline reflects the TRUE per-node outcome -- a node with real evidence
  (completed coder, tombstoned/failed reviewer) shows that evidence; a node
  with no evidence at all AND a flow that never reached it (synthesis) shows
  PENDING, not a fabricated DONE.
- Actual: every evidence-less node defaulted to DONE because the hub's own
  last CHAT TURN had completed normally, even though the FLOW inside it was
  stopped mid-round and never reached that node.

## 5. Root Cause

`resumedFlowStepRows` (interactive_resume.go) seeds every flow node PENDING,
overlays real per-child session evidence (a completed coder -> DONE, a
BUG-319 tombstoned/failed reviewer -> FAILED), then -- ONLY for nodes still
PENDING after that overlay (no evidence at all, e.g. the inline `synthesis`
hub, or a child that was fully dropped pre-BUG-319) -- defaults them to DONE
when `flowComplete` is true. `flowComplete` was
`!resumedFlowRunIncomplete(st) && LoopState.Status != "blocked"`, and
`resumedFlowRunIncomplete` treats a `"stopped"` `LoopState.Status` as terminal
(`terminalFlowLoopStatus`) exactly like a genuinely `"done"` one. Since the
hub's own last chat turn (the user's "return (claude-flow-stop)..."
follow-up) completed normally, `st.Status == Completed`, and the stopped-loop
short-circuit in `resumedFlowRunIncomplete` returned `false` (not
incomplete) -- so `flowComplete` came out `true`, and `synthesis` (no
evidence, hub.inline) defaulted to DONE.

A SEPARATE code path -- the step-transition-log replay branch, used on a
same-machine restart when a local sidecar exists -- had the identical bug via
a DIFFERENT check: `normalizeResumedFlowStatus(st) == RunStatusCompleted`.
That function intentionally returns `Completed` for a stopped loop whose hub
later got a plain-chat follow-up (BUG-308's own contract for the run's
history badge), so it ALSO promoted a still-PENDING hub to DONE for a stopped
flow. The two paths agreed with each other (both wrong) but for unrelated
reasons -- fixing only one would have left a same-machine restart and a
Drive restore of the identical run disagreeing on the display.

## 6. Fix Strategy

- New `resumedFlowStepsComplete(st ProviderSessionState) bool`: returns
  `false` immediately when `LoopState.Status == "stopped"`; otherwise
  identical to the old `flowComplete` expression
  (`!resumedFlowRunIncomplete(st) && LoopState.Status != "blocked"`).
- `resumedFlowStepRows`'s `flowComplete := ...` now calls this helper.
- The transition-log replay's hub-promotion check
  (`if normalizeResumedFlowStatus(st) == RunStatusCompleted`) now calls the
  SAME helper (`if resumedFlowStepsComplete(st)`) instead, so both paths
  agree.
- Nothing else changes: a `"done"` loop still defaults evidence-less nodes to
  DONE (BUG-260 parity, still verified below); a `"blocked"` loop is
  unaffected (already excluded); a legacy run with no loop status at all is
  unaffected (the stopped-only exclusion is the sole behavior change);
  per-child evidence still always wins over the default.

## 7. Validation

- Red-first, focused unit repro (no Drive/tombstone involved --
  `resumedFlowStepRows` exercised directly via `reconstructRun`):
  `TestResumedFlowStepsCompleteStoppedFlowLeavesEvidencelessNodesPending`
  fails pre-fix (`synthesis = "DONE", want PENDING`) then passes; coder=DONE
  and reviewer=FAILED (real evidence) are correct on both sides of the fix,
  proving this is purely a no-evidence-default bug.
- BUG-260 parity (R3 -- P2 must NOT change the done-flow case):
  `TestResumedFlowStepsCompleteDoneFlowStillDefaultsEvidencelessToDone` --
  identical shape with `LoopState.Status == "done"` -- coder DONE, reviewer
  FAILED (evidence wins), synthesis DONE (default fallback still fires).
  Passes before AND after this fix, locking that only the stopped case
  changed.
- Same-machine-restart / transition-log-replay parity (R3):
  `TestResumedFlowStepsCompleteStoppedFlowTransitionLogReplayAgreesWithEvidenceWalk`
  seeds a local step-transition-log sidecar (coder DONE, reviewer FAILED,
  synthesis never logged) and proves the replay branch also leaves synthesis
  PENDING for a stopped loop -- fails pre-fix (`synthesis = "DONE"`), passes
  after, confirming the SAME predicate now governs both code paths.
- Full end-to-end regression proof, tying BUG-319 (tombstone) and this fix
  together exactly as the live repro hit them:
  `TestRestoreThenResumeStoppedFlowShowsReviewerFailedAndSynthesisPending`
  syncs a stopped hub + real completed coder + never-synced (failed)
  reviewer, deletes locally, restores from Drive, then calls the REAL desktop
  code path (`resumeRun`) and asserts `LoadRunSteps`: coder DONE, reviewer
  FAILED, synthesis PENDING.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Existing flow-resume suite re-run unmodified and green: every
  `TestReconstruct*` (including
  `TestReconstructResumeKeepsCompletedFlowNodesAndCancelsSyntheticHub` and
  `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`,
  BUG-260's own guard), `TestStepTransitionReplay*`, BUG-308's
  stopped-plus-completed-history-badge tests, and the full pre-existing
  chat-sync/restore suite -- all green, unmodified.
- Full-package sweep vs `git stash` baseline (BUG-319+320 combined, since
  BUG-319 was never committed separately) at the same HEAD: fix = 2649
  passed / 19 failed; baseline = 2636 passed / 17 failed. The 2 differing
  FAIL names (`TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`,
  `TestRun75035_HubResumeStillLoadsOwnCodexSession`) are pre-existing
  order-dependent flakes in files this change never touches -- both pass 3/3
  in isolation on the fixed tree.
- Real `provider-accounts.json` verified after every sweep; one unrelated
  pre-existing test-isolation gap found and cleaned up (see Known Limits).
- Live end-to-end: PENDING -- the fix loads at runner build/boot; the running
  binary predates it. To verify: rebuild + restart, reproduce the exact
  sequence (stop a flow mid reviewer-start, sync up, delete locally, sync
  down, open the restored chat), confirm the step timeline shows the reviewer
  FAILED and synthesis PENDING (not "Round 1/3, 3/3 steps" all-DONE).

## 8. Regression Guard

- `TestResumedFlowStepsCompleteStoppedFlowLeavesEvidencelessNodesPending` and
  `TestRestoreThenResumeStoppedFlowShowsReviewerFailedAndSynthesisPending`
  lock the core fix for both the direct-reconstruct unit shape and the full
  restore-then-resume end-to-end shape.
- `TestResumedFlowStepsCompleteDoneFlowStillDefaultsEvidencelessToDone` locks
  BUG-260 parity -- a genuinely done flow's evidence-less default is
  unchanged.
- `TestResumedFlowStepsCompleteStoppedFlowTransitionLogReplayAgreesWithEvidenceWalk`
  locks same-machine-restart / Drive-restore display agreement.
- The pre-existing `TestReconstructResumeKeepsCompletedFlowNodesAndCancelsSyntheticHub`,
  `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`,
  `TestStepTransitionReplayKeepsFailedDespiteFlowDone`, and BUG-308's
  stopped-plus-completed tests stay untouched and green.

## 9. Follow-Up Document Updates

- CA-419 records the change.
- Live end-to-end section above needs the user's post-restart retest result.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-320
change_type: bugfix
summary: resumedFlowStepRows and the step-transition-log replay branch now share one resumedFlowStepsComplete(st) predicate that treats a STOPPED flow loop as not-complete (only a genuinely DONE loop still defaults evidence-less steps to DONE), so a restored or same-machine-restarted stopped flow's step timeline shows a failed/canceled cohort member as such and an unreached node (e.g. synthesis) as PENDING instead of fabricating an all-DONE "Round 1/3, 3/3 steps" display; BUG-260's done-flow default and BUG-308's own history-badge contract are both left unchanged.
# --->8---
