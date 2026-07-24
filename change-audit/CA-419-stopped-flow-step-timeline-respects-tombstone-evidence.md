# CA-419: a stopped flow's step timeline no longer defaults evidence-less nodes to DONE

## Summary

Live-verified immediately after BUG-319/CA-418 shipped: restoring the exact
repro chat (a Claude flow-claude review-loop stopped mid-reviewer-turn) now
succeeded, but opening it showed the step-timeline panel as "Round 1/3, 3/3
steps" -- the reviewer and synthesis both DONE, when the source machine's own
flow diagnostic log proved the reviewer actually FAILED and synthesis never
ran at all. Full analysis in
[BUG-320](../requirements/09-BugFix/done/BUG-320-Restored-Stopped-Flow-Shows-Wrong-Step-Timeline-Because-Dropped-Child-Had-No-Evidence.md).

Root cause: `resumedFlowStepRows` (interactive_resume.go) defaults any
flow-step row with NO matching child-session evidence to DONE whenever
`flowComplete` is true. `flowComplete` treated a `LoopState.Status ==
"stopped"` loop exactly like a genuinely `"done"` one (via
`resumedFlowRunIncomplete`/`terminalFlowLoopStatus`), because the hub's own
last CHAT TURN had completed normally even though the FLOW inside it stopped
mid-round. A separate transition-log-replay branch (same-machine restart,
local sidecar present) had the identical bug via a different, unrelated
check (`normalizeResumedFlowStatus(st) == RunStatusCompleted`, which
intentionally reports Completed for a stopped loop with a later plain-chat
follow-up -- BUG-308's own history-badge contract) -- so a Drive restore and
a same-machine restart of the identical run disagreed on the display, both
wrongly.

## Change

- New `resumedFlowStepsComplete(st ProviderSessionState) bool`
  (interactive_resume.go): identical to the old `flowComplete` expression
  except it returns `false` immediately when `LoopState.Status == "stopped"`.
- `resumedFlowStepRows`'s `flowComplete` assignment now calls this helper.
- The transition-log replay's hub-promotion check now calls the SAME helper
  instead of `normalizeResumedFlowStatus(st) == RunStatusCompleted`, so both
  code paths agree for the identical run regardless of whether a local
  sidecar exists.

Design notes: `normalizeResumedFlowStatus` and `terminalFlowLoopStatus`
themselves are untouched -- they still correctly answer "what should the
run's own history badge show" (BUG-308) and "is this loop terminal for
resume-cancel purposes", which are different questions from "did the flow's
own steps really finish". Per-child evidence (a real session, or BUG-319's
tombstone) still always wins over the default regardless of
`resumedFlowStepsComplete` (BUG-260's own guarantee, unchanged). This bug was
only reachable via a restored chat because of BUG-319: before that fix, a
never-synced child hard-failed the entire restore, so nobody could ever open
a restored stopped flow to see the defaulting bug at all.

## Provider parity

Provider-agnostic: `resumedFlowStepRows`/`resumedFlowStepsComplete` operate
purely on `ProviderSessionState`/`LoopState`/flow-node shape, with no
provider branching. The end-to-end regression test reuses BUG-319's
tombstone mechanism, itself already proven provider-agnostic.

## additive-tests-only compliance

New test file (`bug320_stopped_flow_step_timeline_test.go`, 4 test functions)
only. No pre-existing test edited. The full pre-existing flow-resume suite
(`TestReconstruct*`, `TestStepTransitionReplay*`, BUG-308's stopped-history
tests) re-run unmodified and green.

R3 matrix coverage: reported repro (stopped loop, real coder evidence,
tombstoned/failed reviewer evidence, evidence-less synthesis -- direct
`reconstructRun` unit shape); BUG-260 parity (a genuinely DONE loop still
defaults evidence-less nodes to DONE, unchanged); same-machine-restart /
transition-log-replay parity (a local sidecar with coder+reviewer logged but
synthesis never logged agrees with the Drive-restore evidence-walk default);
full end-to-end (BUG-319 tombstone + this fix together, via the real
`resumeRun` desktop code path).

## Verification

- Red-first: `TestResumedFlowStepsCompleteStoppedFlowLeavesEvidencelessNodesPending`
  fails pre-fix (`synthesis = "DONE", want PENDING`) then passes.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Existing flow-resume suite: all green, unmodified (see BUG-320 doc for the
  full list).
- Full-package sweep vs `git stash` baseline (BUG-319+320 combined, since
  BUG-319 was never committed on its own) at the same HEAD: fix = 2649
  passed / 19 failed; baseline = 2636 passed / 17 failed. The 2 differing
  FAIL names are pre-existing order-dependent flakes in files this change
  never touches, both confirmed passing 3/3 in isolation on the fixed tree.
- Real `provider-accounts.json` verified after every sweep; see CA-418 for
  the one unrelated pre-existing test-isolation gap found and cleaned up
  during this same verification pass.

## Not fixed by recent commits

This is a newly-discovered defect (found via the user's own live retest of
BUG-319, the same day) with no prior fix attempt.

## Known limits (documented, out of scope)

- Live end-to-end re-verification is pending a runner rebuild/restart (the
  fix loads at build/boot; the running binary predates it).
- A pre-existing, unrelated test-isolation gap in
  `TestRun20332FlowHubHistoryParityForEveryProvider` (leaks a temp-dir Codex
  account into the real machine's `provider-accounts.json` on a full-suite
  run) was found and the real file cleaned up during this work's regression
  sweeps; flagged separately for its own fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-320
change_type: bugfix
summary: resumedFlowStepRows and the step-transition-log replay branch now share one resumedFlowStepsComplete(st) predicate that excludes a STOPPED loop from the "default evidence-less steps to DONE" rule (a genuinely DONE loop is unaffected -- BUG-260 parity), so a restored or same-machine-restarted stopped flow's step timeline shows a failed/canceled cohort member as such and an unreached node as PENDING instead of a fabricated all-DONE display; BUG-308's own stopped-loop history-badge contract is left unchanged.
# --->8---
