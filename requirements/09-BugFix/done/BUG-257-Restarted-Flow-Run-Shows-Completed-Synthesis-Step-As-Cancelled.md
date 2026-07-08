# BUG-257: Restarted Flow Run Shows Completed Synthesis Step As Cancelled

## Metadata

- Document ID: `BUG-257`
- Title: `Restarted Flow Run Shows Completed Synthesis Step As Cancelled`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Manual E2E Test Guide, Scenario 5)
- Child Documents: `none`
- Related Documents: [BUG-250: Restarted Flow Hub Run Permanently Unresumable Placeholder Session](./BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md), [BUG-251: Restarted Child Agent Shows Permanently Stale Running Status](./BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md), [BUG-256: Restarted Flow Step Timeline Loses Reviewer Statuses When Nodes Share Agent](./BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md)
- Replaces: `none`
- Tags: `agent-flow-engine, resume, step-runtime, loop-state, scenario-5`

## AI Quick View

### Summary

- Found during live CP-36 Scenario 5 verification on `run-7804`: the user let a Review Loop run finish completely (coder → both reviewers → synthesis all `DONE`, loop `status: done`), then restarted the server. On reopen, the `synthesis` (hub) step showed `cancelled` instead of `done`, even though the flow had genuinely finished before the restart.
- Two other repros in the same session (`run-7187`, coder-only done before kill; `run-7482`, reviewers done but synthesis still running at kill) restored correctly, isolating the bug to the specific case of a run that reached full completion.
- Root cause: `sessions.ndjson` is append-only and resume only reads the **latest** record per run. The hub's own synthesis turn persists its snapshot **twice** near the end: once synchronously inside `applyFlowControl`'s `"done"` handling (correct — carries `LoopState.Status: "done"`), and again moments later when the turn itself finishes streaming (`runTurn`'s post-turn persist). That second, later write used `sessionStateOf(rs)` directly, which never populates `LoopState` (it isn't a field on `interactiveRun`; the live value lives in `agentOrchestrator.loop`), so it silently overwrote the terminal `"done"` loop state with a zero value. On restart, `resumedFlowRunIncomplete` saw an empty `LoopState.Status` and treated the completed run as still in-flight, causing the hub/synthesis node (a `hub.inline` node with no child agent run of its own to independently prove `DONE`) to fall through to the incomplete-flow rebuild path and get marked `cancelled`.

### Current Ask

- On restart, a flow run that genuinely reached `loop_state.status == "done"` before the kill must restore every node — including the inline hub/synthesis node — as `DONE`, not `cancelled`.

### Key Decisions

- `V-1` Fix at the persistence layer, not the resume-reconstruction layer: make the post-turn persist in `runTurn` carry forward the live `LoopState` for parent/hub runs, the same way the pre-existing `startTurn` persist, `snapshotWithLoop`, and `persistParentSession` already do. This keeps the already-correct `resumedFlowRunIncomplete`/`resumedFlowStepRows` restore logic untouched and simply stops it from being fed a corrupted last record.

### Constraints

- Must not regress Scenario 1 (kill before reviewers run) or Scenario 2 (kill while synthesis is still in flight) — both must keep restoring the hub node as `cancelled`, since those runs never reached a terminal loop state.
- Only parent/root runs track `LoopState` (`agentOrchestrator.loop` is keyed by parent run id); child agent runs must not attempt to patch it in.

### Open Questions

- none

### Source Refs

- Live feature log: `.flowpilot/logs/features/agent-flow-engine/run-7804.ndjson`
- Session manifest: `.flowpilot/chats/sessions.ndjson`
- [interactive_service.go](../../../apps/local-runner/internal/runner/interactive_service.go)
- [interactive_resume.go](../../../apps/local-runner/internal/runner/interactive_resume.go)
- [interactive_service_e2e_test.go](../../../apps/local-runner/internal/runner/interactive_service_e2e_test.go)

## 1. Issue Summary

After `run-7804` ran a full Review Loop to completion (round 1, both reviewers approved, hub called `submit_review_outcome(done)`, flow marked done) and the server was then restarted, reopening the run showed `coder`/`reviewer_correctness`/`reviewer_security` as `done` but `synthesis` as `cancelled`.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Manual E2E Test Guide Scenario 5
- impacted task: none directly
- impacted tech design: none directly
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner
- flow: built-in Review Loop (`flowpilot-core-flow-pack/review-loop`)
- run: `run-7804`
- reproduction steps:
  1. Start a Review Loop run via the Bug tab's built-in orchestration picker.
  2. Let it run to full completion: coder done, both reviewers done, hub synthesis calls `submit_review_outcome(done)`, board shows `done`.
  3. Turn off / kill the local runner server.
  4. Restart the server and reopen the run.

## 4. Expected vs Actual

- expected: all four steps (`coder`, `reviewer_correctness`, `reviewer_security`, `synthesis`) restore as `done`.
- actual: the first three restore as `done`; `synthesis` restores as `cancelled`.

## 5. Impact

- users affected: users reopening a fully-completed flow-engine run after a runner restart
- workflows affected: any Review Loop / custom flow whose hub node (`hub.inline`, e.g. `synthesis`) has no child agent run of its own — its restored status depends entirely on the persisted `LoopState`
- severity: medium — no data loss and no functional regression (the run really did complete), but the restored timeline falsely reports the hub step as cancelled, which is misleading and inconsistent with the rest of the board

## 6. Root Cause

- confirmed cause: `sessionStateOf(rs)` (`interactive_service.go`) never sets `ProviderSessionState.LoopState` — by design, since `interactiveRun` doesn't carry a `LoopState` field; the live value lives in `agentOrchestrator.loop`, keyed by parent run id. Every call site that needs `LoopState` in the persisted snapshot patches it in afterward (`snapshotWithLoop`, `persistParentSession`, and the `startTurn` persist all do `if isParent { snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState }`). The post-turn persist inside `runTurn` (fires after every provider turn via `finishTurn`) was missing this patch.
- mechanism: `applyFlowControl`'s `"done"` branch persists a correct record (`loop_state.status: "done"`) via `go s.persistParentSession(parentRunID)`, fired synchronously from inside the tool call, before the enclosing provider turn itself finishes. Seconds later (the real turn keeps streaming after the tool call in production; confirmed in `sessions.ndjson` as a ~4s gap between the two records), the turn completes and `runTurn`'s post-turn persist writes again — this time with a zero-value `LoopState` — becoming the new latest record.
- downstream effect: on restart, `resumedFlowRunIncomplete(st)` reads `st.LoopState.Status == ""` (not a terminal value) and, since `st.AutoOrchestrate` was still `true` on that same corrupted record, falls through to `return true` (treats the run as incomplete). `resumedFlowStepRows` then takes the incomplete-flow rebuild path instead of the fast "all nodes DONE" path. Only nodes with a matching child agent session get marked `DONE` there; the inline hub node (`synthesis`, `behavior: hub.inline`, no child run of its own) has no such evidence and falls through to the unconditional-cancel branch (`hubInlineNodeID` block), which marks any still-`PENDING` hub node `cancelled` whenever `LoopState.ActiveNode == hubID` or a joined-review note is present — both true here since the corrupted record still carried the stale `ActiveNode`/pending-context fields from before completion.

## 7. Fix Strategy

- `F-1` In `runTurn`'s post-turn persist (`interactive_service.go`), capture `isParent := rs.parentRunID == ""` under the lock alongside the existing snapshot, then — mirroring `startTurn`'s identical patch — set `snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState` for parent/hub runs before calling `persistProviderSession`. This makes the trailing turn-finish write carry forward whatever terminal (or in-progress) loop state already exists instead of silently zeroing it.
- No change to `resumedFlowRunIncomplete`/`resumedFlowStepRows`/`hubInlineNodeID` — once the persisted record is no longer corrupted, the existing restore logic already does the right thing (fast "all DONE" path for a genuinely terminal `LoopState.Status`).

## 8. Validation

- `rtk go test ./internal/runner -run 'TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes'` — passed with the fix; confirmed to fail (`persisted LoopState.Status = "", want done`) against the pre-fix code via a temporary revert.
- `rtk go test ./internal/runner -run 'TestReconstruct|TestE2EReviewLoop|TestApplyFlowControl|TestMarkFlowRunComplete|TestCohort|TestSubmitFlowControl' -count=1` — 45 passed (no regression to Scenario 1/2/9-style restart restores, or to the CA-251/252/253 fixes).
- `rtk go test ./internal/runner/... -count=1` — same pre-existing unrelated failures on both baseline and fixed code (Codex CLI unavailable in this environment, Windows home-dir/path fixtures, skill-merge fixtures); no new failures introduced.

## 9. Regression Guard

- tests:
  - `TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes` (new) — drives a full Review Loop to completion against a real `LocalFileSessionStore` and asserts the latest persisted record's `LoopState.Status == "done"`, i.e. exactly what a restart's resume path reads.
- impact analysis:
  - `rtk npx gitnexus impact --repo flowpilot runTurn` — `LOW`.
  - `rtk npx gitnexus impact --repo flowpilot sessionStateOf` — `HIGH` fan-in overall, but this change does not modify `sessionStateOf` itself; it only adds the same post-call `LoopState` patch already applied at three other call sites (`snapshotWithLoop`, `persistParentSession`, `startTurn`), so `sessionStateOf`'s own return value and every other caller are unaffected.

## 10. Follow-Up Document Updates

- upstream docs updated: CP-36 Scenario 5 note should reference this fix for the "reviewers had completed / full completion before restart" case.
