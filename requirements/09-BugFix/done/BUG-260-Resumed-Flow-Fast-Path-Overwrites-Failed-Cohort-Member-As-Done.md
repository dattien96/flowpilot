# BUG-260: Resumed Flow Fast Path Overwrites Failed Cohort Member As Done

## Metadata

- Document ID: `BUG-260`
- Title: `Resumed Flow Fast Path Overwrites Failed Cohort Member As Done`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Manual E2E Test Guide, Scenario 9)
- Child Documents: `none`
- Related Documents: [BUG-254: Failed Cohort Reviewer Mark Done And Hidden Node Identity](./BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md), [BUG-256: Restarted Flow Step Timeline Loses Reviewer Statuses When Nodes Share Agent](./BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md), [BUG-257: Restarted Flow Run Shows Completed Synthesis Step As Cancelled](./BUG-257-Restarted-Flow-Run-Shows-Completed-Synthesis-Step-As-Cancelled.md)
- Replaces: `none`
- Tags: `agent-flow-engine, resume, step-runtime, cohort, scenario-9, claude`

## AI Quick View

### Summary

- Live repro (`run-10239`, Claude provider, custom two-reviewer flow): a cohort of two reviewers ran, one completed (`my-reviewer`) and one genuinely failed (`claude-review-fake-model`, deliberately configured with an invalid model). Live behavior was correct — the failed step showed `failed`, the flow proceeded off the surviving reviewer's verdict, board looked right. After restarting the server and reopening the run, the failed reviewer's step incorrectly showed `done`.
- Not a Claude-vs-Codex difference (the owner suspected one) — the same defect reproduces identically for either provider once the specific condition is met: a cohort with one failed member, where the hub still reaches a genuinely terminal `loop_state.status: "done"` afterward (CA-251/BUG-254 deliberately allows the hub to proceed off surviving cohort members, so this is an expected, supported outcome, not a fluke).
- Root cause: `resumedFlowStepRows` (`interactive_resume.go:539`) had a fast path — `if flow reached a terminal, non-blocked loop state: return every node as DONE` — that fired *before* the careful per-child evidence walk that already correctly distinguishes `DONE`/`FAILED`/`CANCELED` per session. That fast path was written for (and only tested against) the case where "flow reached done" implied "every node genuinely succeeded" (BUG-257's exact scenario). CA-251/BUG-254 made that implication false: the hub can legitimately declare a round done with a failed cohort member still in the mix. The fast path never consulted per-child evidence, so it silently overwrote the failed reviewer's real outcome.

### Current Ask

- On restart, a flow that reached genuine completion but had one or more individually-failed cohort members must restore those members as `FAILED`, not `DONE` — regardless of the overall flow's terminal loop status.

### Key Decisions

- `V-1` Unify the two restore paths: always run the per-child session-evidence walk (DONE/FAILED/CANCELED per node) first. The previous fast path's "everything DONE" behavior becomes a *default fallback* applied only to nodes that still have no session evidence at all after that walk (e.g. an inline hub node with no child run of its own, or a legacy run with no persisted children) — and only when the flow's loop state is confirmed terminal. Per-child evidence, including `FAILED`, always wins over that fallback.
- This keeps BUG-257's original scenario correct (a hub/synthesis node with no child session still defaults to `DONE` when the flow genuinely completed) while fixing this bug (a reviewer node *with* child session evidence keeps its real `FAILED` outcome).

### Constraints

- Must not regress BUG-257 (Scenario 5's full-completion case: every node DONE, including the inline hub with no child session).
- Must not regress BUG-256/CA-253 (label-based and legacy-cohort-order node matching) or the existing blocked/incomplete restore paths (`TestReconstructResumeKeepsCompletedFlowNodesAndCancelsSyntheticHub`, `TestReconstructResumeCancelsRunningReviewerButKeepsCompletedCoder`).

### Open Questions

- None.

### Source Refs

- Live repro: `run-10239` (Claude), cohort `flow-auto-my-coder-round-0` with `run-10331` (`my-reviewer`, completed) and `run-10339` (`claude-review-fake-model`, failed) — `.flowpilot/chats/sessions.ndjson`.
- [interactive_resume.go](../../../apps/local-runner/internal/runner/interactive_resume.go) — `resumedFlowStepRows`.
- [flow_step_runtime_test.go](../../../apps/local-runner/internal/runner/flow_step_runtime_test.go) — `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`.

## 1. Issue Summary

After a custom Review Loop flow's cohort had one reviewer fail and one complete, and the hub still resolved the round to a genuinely terminal `done` loop state, restarting the server and reopening the run showed the failed reviewer's step as `done` instead of `failed`.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Manual E2E Test Guide Scenario 9
- impacted task: none directly
- impacted tech design: none directly
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner, Claude provider
- flow: custom two-reviewer flow (`my-coder` → `my-reviewer` + `claude-review-fake-model` cohort → synthesis), one reviewer deliberately configured with an invalid model so it fails
- run: `run-10239`
- reproduction steps:
  1. Start the flow; let the coder complete.
  2. Both reviewers run in the cohort; `claude-review-fake-model` fails (invalid model), `my-reviewer` completes normally.
  3. The hub escalates on the partial failure, resolves, and the loop reaches `status: "done"`.
  4. Restart the server and reopen the run.

## 4. Expected vs Actual

- expected: `my-coder` and `my-reviewer` restore as `done`; `claude-review-fake-model` restores as `failed`; synthesis restores as `done`.
- actual: all four nodes, including `claude-review-fake-model`, restored as `done`.

## 5. Impact

- users affected: anyone reopening a flow run after a restart where a cohort member failed but the flow still reached genuine completion (any provider — not Claude-specific)
- workflows affected: any custom or built-in Review Loop flow whose cohort can partially fail while still letting the hub resolve the round
- severity: medium — no data loss (the underlying failure and transcript are intact), but the restored timeline actively hides that a reviewer failed, which is exactly the information BUG-254 fixed for the *live* case

## 6. Root Cause

- confirmed cause: `resumedFlowStepRows`'s fast path (`interactive_resume.go:543` pre-fix) — `if !resumedFlowRunIncomplete(st) && st.LoopState.Status != "blocked": return all nodes DONE` — ran unconditionally whenever the flow's own loop state was terminal and non-blocked, entirely bypassing the per-child session-evidence walk (`matchFlowNodeForSession` / `resumedChildRunStepStatus`) that already correctly reads `FAILED` from a reviewer's own persisted session. That per-child walk only ever ran in the *other* branch (loop state incomplete or blocked) — a branch this repro's terminal `"done"` state never reaches.
- why this looked new: BUG-257 (fixed the same week) made a genuinely-completed flow's *loop state* reliably persist as `"done"` across a restart. Before that fix, a completed flow's last persisted record often had a corrupted/empty loop state, which routed resume through the *other* (per-child-evidence) branch by accident — masking this fast-path defect. Fixing BUG-257 correctly restored `loop_state.status: "done"`, which now reliably routes a genuinely-completed flow with a failed cohort member into the buggy fast path for the first time.

## 7. Fix Strategy

- `F-1` In `resumedFlowStepRows`, replace the early-return fast path with a `flowComplete` boolean computed the same way, but always run the per-child session-evidence walk first (unconditionally, not just in the "incomplete" branch).
- `F-2` The inline hub node's fallback logic becomes: if `flowComplete`, default to `DONE`; else if `ActiveNode == hubID` or a joined-review note exists, default to `CANCELED`.
- `F-3` After the per-child walk, if `flowComplete`, sweep any node *still* `PENDING` (no session evidence matched at all) to `DONE` — this is the fallback that preserves BUG-257's original scenario, but only for nodes with zero evidence; a node already set to `FAILED`/`CANCELED`/`DONE` by real evidence is untouched.

## 8. Validation

- `rtk go test ./internal/runner -run 'TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone' -v -count=1` — passed; confirmed to fail against the pre-fix fast path via a temporary scoped revert (`claude-review-fake-model = "DONE", want FAILED`).
- `rtk go test ./internal/runner -run 'TestReconstruct' -v -count=1` — all 10 `TestReconstruct*` tests passed, confirming no regression to BUG-257/BUG-256's own restore scenarios.
- `rtk go test ./internal/runner/... -count=1` — 1144 passed (was 1143 before this fix's new test), same 15 pre-existing unrelated failures; no new failures.
- `rtk npx gitnexus impact --repo flowpilot resumedFlowStepRows` — `LOW`.

## 9. Regression Guard

- tests:
  - `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone` (new) — a cohort with one completed and one failed reviewer, parent loop state `"done"`; asserts the failed reviewer restores as `FAILED`, the completed reviewer and coder as `DONE`, and the inline hub node (no child session) as `DONE`.
  - existing `TestReconstructResumeKeepsCompletedFlowNodesAndCancelsSyntheticHub`, `TestReconstructResumeCancelsRunningReviewerButKeepsCompletedCoder`, `TestReconstructWorkflowRunRestoresStepTimeline`, `TestReconstructNonCompletedFlowRunRestoresPendingSteps`, `TestReconstructIncompleteCompletedFlowRunCancelsResumeAndDisablesAutoOrchestrate` all continue to pass unchanged.
- impact analysis:
  - `rtk npx gitnexus impact --repo flowpilot resumedFlowStepRows` — `LOW`.

## 10. Follow-Up Document Updates

- upstream docs updated: CP-36 Scenario 9 should note this restart-specific gap alongside the live-case fix already recorded there (BUG-254/CA-251).
